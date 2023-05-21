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

package router

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RouteReconciler reconciles a Router object
type RouteReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	req                  *reconcile.Request
	logger               logr.Logger
	tlsSecretName        string
	globalTlsSecretName  string
	RouterToContourRatio int
	Route                *routev1.Route
	Httpproxy            *contourv1.HTTPProxy
}

//+kubebuilder:rbac:groups=router.openshift.io,resources=routers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=router.openshift.io,resources=routers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=router.openshift.io,resources=routers/finalizers,verbs=update

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)
	r.req = &req

	subrecs := []subreconciler.Fn{
		r.findRoute,
		r.initVars,
		r.ensureHttpproxy,
	}

	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *RouteReconciler) findRoute(ctx context.Context) (*ctrl.Result, error) {
	if err := r.Get(ctx, r.req.NamespacedName, r.Route); err != nil {
		r.logger.Error(err, "failed to get route")
		return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
	}

	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) initVars(ctx context.Context) (*ctrl.Result, error) {
	r.tlsSecretName = fmt.Sprintf("%s-tls", r.Route.Name)
	r.globalTlsSecretName = fmt.Sprintf("%s/%s", consts.TLSSecretNS, consts.TLSSecretName)

	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) ensureHttpproxy(ctx context.Context) (*ctrl.Result, error) {
	switch err := r.Get(ctx, types.NamespacedName{Namespace: r.Route.Namespace, Name: r.Route.Name}, r.Httpproxy); {
	case err == nil:
		return r.updateHttpproxy(ctx)
	case errors.IsNotFound(err):
		return r.createHttpproxy(ctx)
	default:
		r.logger.Error(err, "failed to get desired")
		return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
	}
}

func (r *RouteReconciler) updateHttpproxy(ctx context.Context) (*ctrl.Result, error) {
	desired, err := r.assembleHttpproxy(ctx)
	if err != nil {
		r.logger.Error(err, "failed to convert route to desired")
		return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
	}

	if !reflect.DeepEqual(r.Httpproxy.Spec, desired.Spec) {
		r.Httpproxy.Spec = desired.Spec
		err = r.Update(ctx, r.Httpproxy)
		if err != nil {
			r.logger.Error(err, "failed to update httpproxy", "namespace", desired.Namespace, "name", desired.Name)
			return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) createHttpproxy(ctx context.Context) (*ctrl.Result, error) {
	desired, err := r.assembleHttpproxy(ctx)
	if err != nil {
		r.logger.Error(err, "failed to convert route to desired")
		return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
	}

	if err = r.Create(ctx, desired); err != nil {
		r.logger.Error(err, "failed to create httpproxy", "namespace", desired.Namespace, "name", desired.Name)
		return subreconciler.RequeueWithDelayAndError(consts.DefaultRequeueTime, err)
	}

	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) assembleHttpproxy(ctx context.Context) (*contourv1.HTTPProxy, error) {
	httpproxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Route.Name,
			Namespace: r.Route.Namespace,
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: r.Route.Spec.Host,
			},
		},
	}

	httpproxy.Spec.IngressClassName = getIngressClass(r.Route)

	if r.Route.Spec.TLS != nil {
		switch r.Route.Spec.TLS.Termination {
		case "passthrough":
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{
				Passthrough: true,
			}
		case "edge", "reencrypt":
			var secretName string
			if r.Route.Spec.TLS.Key != "" {
				err := r.ensureTLSSecret(ctx)
				if err != nil {
					return nil, err
				}
				secretName = r.tlsSecretName
			} else {
				secretName = r.globalTlsSecretName
			}
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{
				SecretName:                secretName,
				EnableFallbackCertificate: true,
			}
		default:
			return nil, fmt.Errorf("invalid termination mode specified on route")
		}
	}

	rateLimitEnabled, rateLimit := getRateLimit(r.Route)
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

	loadBalancerPolicy, err := getLoadBalancerPolicy(r.Route)
	if err != nil {
		return nil, err
	}
	ports, err := r.getTargetPorts(ctx, r.Route)
	if err != nil {
		return nil, fmt.Errorf("failed to get route target port, %v", err)
	}

	for _, port := range ports {
		httpproxy.Spec.Routes = []contourv1.Route{
			{
				Services: []contourv1.Service{
					{
						Name:   r.Route.Spec.To.Name,
						Port:   port,
						Weight: int64(pointer.Int32Deref(r.Route.Spec.To.Weight, 1)),
					},
				},
				LoadBalancerPolicy: loadBalancerPolicy,
				TimeoutPolicy: &contourv1.TimeoutPolicy{
					Response: getTimeout(r.Route),
				},
				EnableWebsockets: true,
			},
		}

		if r.Route.Spec.Path != "" {
			httpproxy.Spec.Routes[0].Conditions = []contourv1.MatchCondition{
				{Prefix: r.Route.Spec.Path},
			}
		}

		if r.Route.Spec.TLS != nil {
			if r.Route.Spec.TLS.InsecureEdgeTerminationPolicy == routev1.InsecureEdgeTerminationPolicyAllow {
				httpproxy.Spec.Routes[0].PermitInsecure = true
			}
		}

		ipWhitelist := getIPWhitelist(r.Route)
		if len(ipWhitelist) > 0 {
			httpproxy.Spec.Routes[0].IPAllowFilterPolicy = ipWhitelist
		}
	}

	if err := ctrl.SetControllerReference(r.Route, httpproxy, r.Scheme); err != nil {
		return nil, err
	}

	return httpproxy, nil
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

	svc := corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: route.Namespace, Name: route.Spec.To.Name}, &svc); err != nil {
		return ports, err
	}

	for _, port := range svc.Spec.Ports {
		if port.Name == targetPortName || port.Port == int32(targetPort) {
			ports = []int{int(port.Port)}
			return ports, nil
		}
		ports = append(ports, int(port.Port))
	}

	return ports, nil
}

func (r *RouteReconciler) ensureTLSSecret(ctx context.Context) error {
	existingSecret := corev1.Secret{}

	switch err := r.Get(ctx, types.NamespacedName{
		Namespace: r.Route.Namespace,
		Name:      r.tlsSecretName,
	}, &existingSecret); {
	case err == nil:
		secret := r.assembleTLSSecret()
		if !reflect.DeepEqual(secret.Data, existingSecret.Data) {
			existingSecret.Data = secret.Data
			return r.Update(ctx, &existingSecret)
		}
		return nil
	case errors.IsNotFound(err):
		secret := r.assembleTLSSecret()
		return r.Create(ctx, secret)
	default:
		return err
	}
}

func (r *RouteReconciler) assembleTLSSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.Route.Namespace,
			Name:      r.tlsSecretName,
		},
		StringData: map[string]string{
			"tls.key": r.Route.Spec.TLS.Key,
			"tls.crt": r.Route.Spec.TLS.Certificate,
		},
		Type: corev1.SecretTypeTLS,
	}
}

func getLoadBalancerPolicy(route *routev1.Route) (*contourv1.LoadBalancerPolicy, error) {
	lbPolicy := contourv1.LoadBalancerPolicy{}
	disableCookies := route.Annotations[consts.AnnotDisableCookies]
	if disableCookies != "true" && disableCookies != "TRUE" {
		lbPolicy.Strategy = consts.StrategyCookie
	} else {
		policy, ok := route.Annotations[consts.AnnotBalance]
		if !ok {
			lbPolicy.Strategy = consts.StrategyDefault
		} else {
			switch policy {
			case "roundrobin":
				lbPolicy.Strategy = consts.StrategyRoundRobin
			case "leastconn":
				lbPolicy.Strategy = consts.StrategyWeightedLeastRequest
			case "source":
				lbPolicy.Strategy = consts.StrategyRequestHash
				lbPolicy.RequestHashPolicies = []contourv1.RequestHashPolicy{{HashSourceIP: true}}
			case "random":
				lbPolicy.Strategy = consts.StrategyRandom
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
