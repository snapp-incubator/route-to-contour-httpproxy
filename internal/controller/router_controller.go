/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	corev1 "k8s.io/api/core/v1"
	disoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RouteReconciler reconciles a Router object
type RouteReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	RouterToContourRatio int
}

//+kubebuilder:rbac:groups=router.openshift.io,resources=routers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=router.openshift.io,resources=routers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=router.openshift.io,resources=routers/finalizers,verbs=update

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	requeueResponse := ctrl.Result{RequeueAfter: consts.DefaultRequeueTime}

	route := &routev1.Route{}
	err := r.Get(ctx, req.NamespacedName, route)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "failed to get route")
		return requeueResponse, err
	}

	existingHttpproxy := &contourv1.HTTPProxy{}
	err = r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, existingHttpproxy)
	switch {
	case err == nil:
		desiredHttpproxy, err := r.httpproxyForRoute(ctx, route)
		if err != nil {
			logger.Error(err, "failed to convert route to desiredHttpproxy")
			return requeueResponse, nil
		}
		if !reflect.DeepEqual(existingHttpproxy.Spec, desiredHttpproxy.Spec) {
			err = r.Update(ctx, desiredHttpproxy)
			if err != nil {
				logger.Error(err, "failed to update desiredHttpproxy", "namespace", desiredHttpproxy.Namespace, "name", desiredHttpproxy.Name)
				return requeueResponse, err
			}
		}
		return ctrl.Result{}, nil
	case errors.IsNotFound(err):
		desiredHttpproxy, err := r.httpproxyForRoute(ctx, route)
		if err != nil {
			logger.Error(err, "failed to convert route to desiredHttpproxy")
			return requeueResponse, nil
		}
		err = r.Create(ctx, desiredHttpproxy)
		if err != nil {
			logger.Error(err, "failed to create desiredHttpproxy", "namespace", desiredHttpproxy.Namespace, "name", desiredHttpproxy.Name)
			return requeueResponse, err
		}
		return ctrl.Result{}, nil
	default:
		logger.Error(err, "failed to get desiredHttpproxy")
		return requeueResponse, err
	}
}

func (r *RouteReconciler) httpproxyForRoute(ctx context.Context, route *routev1.Route) (*contourv1.HTTPProxy, error) {
	httpproxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      route.Name,
			Namespace: route.Namespace,
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: route.Spec.Host,
			},
		},
	}

	httpproxy.Spec.IngressClassName = getIngressClass(route)

	if route.Spec.TLS != nil {
		switch route.Spec.TLS.Termination {
		case "passthrough":
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{
				Passthrough: true,
			}
		case "edge":
			var secretName string
			if route.Spec.TLS.Key == "" {
				// use default secret
				secretName = fmt.Sprintf("%s/%s", consts.TLSSecretNS, consts.TLSSecretName)
			} else {
				// create secret from route
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-tls", route.Name),
						Namespace: route.Namespace,
					},
					StringData: map[string]string{
						"tls.key": route.Spec.TLS.Key,
						"tls.crt": route.Spec.TLS.Certificate,
					},
					Type: corev1.SecretTypeTLS,
				}
				if err := r.Create(ctx, &secret); err != nil {
					return nil, fmt.Errorf("failed to create secret for tls, %v", err)
				}
				secretName = secret.Name
			}
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{SecretName: secretName}
		case "reencrypt":
			// todo: find a solution
			return nil, fmt.Errorf("reencrypt termination is not supported")
		default:
			return nil, fmt.Errorf("invalid termination mode specified on route")
		}
	}

	rateLimitEnabled, rateLimit := getRateLimit(route)
	if rateLimitEnabled {
		// haproxy router uses 10s window size while we have 1s, 1m, 1h windows in contour
		contourRate := uint32(rateLimit * r.RouterToContourRatio * 6)

		httpproxy.Spec.VirtualHost.RateLimitPolicy = &contourv1.RateLimitPolicy{
			Local: &contourv1.LocalRateLimitPolicy{
				Requests: contourRate,
				Unit:     consts.RateLimitUnitMinute,
			},
		}
	}

	loadBalancerPolicy, err := getLoadBalancerPolicy(route)
	if err != nil {
		return nil, err
	}
	ports, err := r.getTargetPorts(ctx, route)
	if err != nil {
		return nil, fmt.Errorf("failed to get route target port, %v", err)
	}

	for _, port := range ports {
		httpproxy.Spec.Routes = []contourv1.Route{
			{
				Services: []contourv1.Service{
					{
						Name:   route.Spec.To.Name,
						Port:   port,
						Weight: int64(pointer.Int32Deref(route.Spec.To.Weight, 1)),
					},
				},
				LoadBalancerPolicy: loadBalancerPolicy,
				TimeoutPolicy: &contourv1.TimeoutPolicy{
					Response: getTimeout(route),
				},
				EnableWebsockets: shouldEnableWebsockets(route),
			},
		}

		if route.Spec.Path != "" {
			httpproxy.Spec.Routes[0].Conditions = []contourv1.MatchCondition{
				{Prefix: route.Spec.Path},
			}
		}

		if route.Spec.TLS != nil {
			if route.Spec.TLS.InsecureEdgeTerminationPolicy == routev1.InsecureEdgeTerminationPolicyAllow {
				httpproxy.Spec.Routes[0].PermitInsecure = true
			}
		}

		ipWhitelist := getIPWhitelist(route)
		if len(ipWhitelist) > 0 {
			httpproxy.Spec.Routes[0].IPAllowFilterPolicy = ipWhitelist
		}
	}

	if err := ctrl.SetControllerReference(route, httpproxy, r.Scheme); err != nil {
		return nil, err
	}

	return httpproxy, nil
}

func shouldEnableWebsockets(route *routev1.Route) bool {
	return route.Labels[consts.EnableWebsocketsLabel] == "true"
}

func getIngressClass(route *routev1.Route) string {
	ingressClass, ok := route.Labels[consts.RouteShardLabel]
	if !ok {
		ingressClass = consts.DefaultIngressClassName
	}
	return ingressClass
}

func getIPWhitelist(route *routev1.Route) []contourv1.IPFilterPolicy {
	whitelist, ok := route.Annotations[consts.AnnotIPWhitelist]
	if !ok {
		return []contourv1.IPFilterPolicy{}
	}
	whitelistCIDRs := strings.Split(whitelist, " ")
	filterPolicies := make([]contourv1.IPFilterPolicy, len(whitelistCIDRs))
	for _, cidr := range whitelistCIDRs {
		filterPolicies = append(filterPolicies, contourv1.IPFilterPolicy{
			Source: contourv1.IPFilterSourcePeer,
			CIDR:   cidr,
		})
	}
	return filterPolicies
}

func getTimeout(route *routev1.Route) string {
	timeout := consts.HAProxyDefaultTimeout
	if routeTimeout, ok := route.Annotations[consts.AnnotTimeout]; ok {
		timeout = routeTimeout
	}
	return timeout
}

func (r *RouteReconciler) getTargetPorts(ctx context.Context, route *routev1.Route) ([]int, error) {
	var ports []int

	targetPortName := ""
	targetPort := 0
	if route.Spec.Port != nil {
		targetPortName = route.Spec.Port.TargetPort.String()
		targetPort = route.Spec.Port.TargetPort.IntValue()
	}

	endpointSliceList := disoveryv1.EndpointSliceList{}
	err := r.List(ctx, &endpointSliceList, client.MatchingLabels{
		disoveryv1.LabelServiceName: route.Spec.To.Name,
	})
	if err != nil {
		return ports, err
	}

	for _, endpointSlice := range endpointSliceList.Items {
		for _, port := range endpointSlice.Ports {
			if (*port.Name == targetPortName) || (*port.Port == int32(targetPort)) {
				ports = []int{int(*port.Port)}
				return ports, nil
			}
			ports = append(ports, int(*port.Port))
		}
	}

	return ports, nil
}

func getLoadBalancerPolicy(route *routev1.Route) (*contourv1.LoadBalancerPolicy, error) {
	lbPolicy := contourv1.LoadBalancerPolicy{}
	disableCookies := route.Annotations[consts.AnnotDisableCookies]
	if disableCookies != "true" && disableCookies != "TRUE" {
		lbPolicy.Strategy = "Cookie"
	} else {
		policy, ok := route.Annotations[consts.AnnotBalance]
		if !ok {
			lbPolicy.Strategy = "Random"
		} else {
			switch policy {
			case "roundrobin":
				lbPolicy.Strategy = "RoundRobin"
			case "leastconn":
				lbPolicy.Strategy = "WeightedLeastRequest"
			case "source":
				lbPolicy.Strategy = "RequestHash"
				lbPolicy.RequestHashPolicies = []contourv1.RequestHashPolicy{{HashSourceIP: true}}
			case "random":
				lbPolicy.Strategy = "Random"
			default:
				return nil, fmt.Errorf("invalid loadbalancer policy specified on route")
			}
		}
	}

	return &lbPolicy, nil
}

func getRateLimit(route *routev1.Route) (bool, int) {
	rateLimitAnnotationValue := route.Annotations[consts.AnnotRateLimit]
	rateLimit := rateLimitAnnotationValue == "true" || rateLimitAnnotationValue == "TRUE"
	var (
		rateLimitHttpRate int
		err               error
	)
	if rateLimit {
		rateLimitHttpRate, err = strconv.Atoi(route.Annotations[consts.AnnotRateLimitHttpRate])
		if err != nil {
			rateLimit = false
		}
	}

	return rateLimit, rateLimitHttpRate
}

func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&routev1.Route{}).
		Owns(&contourv1.HTTPProxy{}).
		Complete(r)
}
