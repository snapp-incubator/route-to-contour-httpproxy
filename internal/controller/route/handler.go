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

package route

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/utils"
)

// RouteReconciler reconciles a Router object
type RouteReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	RouterToContourRatio int
	Route                *routev1.Route
	Httpproxy            *contourv1.HTTPProxy
	req                  *reconcile.Request
	logger               logr.Logger
}

// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes/finalizers,verbs=update
// +kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)
	r.req = &req
	r.Httpproxy = nil
	r.Route = &routev1.Route{}

	err := r.Get(ctx, r.req.NamespacedName, r.Route)

	if err != nil {
		if errors.IsNotFound(err) {
			return subreconciler.Evaluate(subreconciler.DoNotRequeue())
		}
		r.logger.Error(err, "failed to get route")
		return subreconciler.Evaluate(subreconciler.Requeue())
	}

	if utils.IsPaused(r.Route) {
		r.logger.Info("ignoring paused route")
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	}

	isAdmitted, hasAdmissionStatus := utils.IsAdmitted(r.Route)
	if !hasAdmissionStatus {
		r.logger.Info("ignoring route without status")
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	}

	var subrecs []subreconciler.Fn
	if !isAdmitted || utils.IsDeleted(r.Route) {
		subrecs = append(subrecs,
			r.findHTTPProxybyOwner,
			r.handleRouteCleanup,
			r.removeTLSSecret,
			r.removeRouteFinalizer,
		)
	} else {
		subrecs = append(subrecs,
			r.findHTTPProxybyOwner,
			r.handleHostMismatch,
			r.handleRoute,
			r.addRouteFinalizer,
		)
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}
	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *RouteReconciler) findHTTPProxybyOwner(ctx context.Context) (*ctrl.Result, error) {
	httpproxyList := contourv1.HTTPProxyList{}
	if err := r.List(ctx, &httpproxyList, client.InNamespace(r.Route.Namespace)); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to list httpproxies: %w", err))
	}

	for _, httpproxy := range httpproxyList.Items {
		for _, ownerRef := range httpproxy.GetOwnerReferences() {
			if ownerRef.UID == r.Route.UID {
				r.Httpproxy = &httpproxy
				return subreconciler.ContinueReconciling()
			}
		}
	}
	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) handleRouteCleanup(ctx context.Context) (*ctrl.Result, error) {
	if r.Httpproxy == nil {
		return subreconciler.ContinueReconciling()
	}

	sameHostRoutes, err := r.getSameHostRoutes(ctx, r.Httpproxy.Namespace, r.Httpproxy.Spec.VirtualHost.Fqdn)
	if err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to get routes with the same host: %w", err))
	}
	if len(sameHostRoutes) == 0 {
		if err := r.Delete(ctx, r.Httpproxy); err != nil {
			return subreconciler.RequeueWithError(fmt.Errorf("failed to delete httpproxy: %w", err))
		}
		return subreconciler.ContinueReconciling()
	}

	// remove current object from ownerReferences and set the oldest route as owner
	if err := utils.RemoveOwnerReference(r.Route, r.Httpproxy, r.Scheme); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to remove owner reference: %w", err))
	}
	if err := utils.SetControllerReference(&sameHostRoutes[0], r.Httpproxy, r.Scheme); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to set controller owner reference: %w", err))
	}

	httpproxy, err := r.assembleHttpproxy(ctx, &sameHostRoutes[0], sameHostRoutes)
	if err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to assemble httpproxy: %w", err))
	}

	httpproxy.ObjectMeta = *r.Httpproxy.ObjectMeta.DeepCopy()

	if err := r.Update(ctx, httpproxy); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to update httpproxy: %w", err))
	}
	return subreconciler.ContinueReconciling()
}

// handleHostMismatch will call handleRouteCleanup if the fqdn of the httpproxy doesn't
// match the host of the route
func (r *RouteReconciler) handleHostMismatch(ctx context.Context) (*ctrl.Result, error) {
	if r.Httpproxy == nil {
		return subreconciler.ContinueReconciling()
	}

	if r.Httpproxy.Spec.VirtualHost != nil &&
		r.Httpproxy.Spec.VirtualHost.Fqdn != r.Route.Spec.Host {
		return r.handleRouteCleanup(ctx)
	}

	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) handleRoute(ctx context.Context) (*ctrl.Result, error) {
	found := r.Httpproxy != nil

	if !found {
		httpproxyList := contourv1.HTTPProxyList{}
		if err := r.List(ctx, &httpproxyList, client.InNamespace(r.Route.Namespace)); err != nil {
			return nil, fmt.Errorf("failed to list httpproxies: %w", err)
		}
		for _, httpproxy := range httpproxyList.Items {
			if httpproxy.Spec.VirtualHost != nil && httpproxy.Spec.VirtualHost.Fqdn == r.Route.Spec.Host {
				r.Httpproxy = &httpproxy
				found = true
				break
			}
		}
	}

	sameHostRoutes, err := r.getSameHostRoutes(ctx, r.Route.Namespace, r.Route.Spec.Host)
	if err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to get routes with the same host: %w", err))
	}
	if len(sameHostRoutes) == 0 {
		if found {
			if err := r.Delete(ctx, r.Httpproxy); err != nil {
				return subreconciler.RequeueWithError(fmt.Errorf("failed to delete httpproxy: %w", err))
			}
		}
		return subreconciler.ContinueReconciling()
	}

	httpproxy, err := r.assembleHttpproxy(ctx, &sameHostRoutes[0], sameHostRoutes)
	if err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to assemble httpproxy spec: %w", err))
	}

	if found {
		httpproxy.ObjectMeta = *r.Httpproxy.ObjectMeta.DeepCopy()
	}

	if err := controllerutil.SetOwnerReference(r.Route, httpproxy, r.Scheme); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to set owner reference on httpproxy : %w", err))
	}
	if err := utils.SetControllerReference(&sameHostRoutes[0], httpproxy, r.Scheme); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to set controller owner reference: %w", err))
	}

	if found {
		if err := r.Update(ctx, httpproxy); err != nil {
			return subreconciler.RequeueWithError(err)
		}
		return subreconciler.ContinueReconciling()
	} else {
		if err := r.Create(ctx, httpproxy); err != nil {
			return subreconciler.RequeueWithError(err)
		}
		return subreconciler.ContinueReconciling()
	}
}

func (r *RouteReconciler) addRouteFinalizer(ctx context.Context) (*ctrl.Result, error) {
	updated := controllerutil.AddFinalizer(r.Route, utils.RouteFinalizer)
	if !updated {
		return subreconciler.ContinueReconciling()
	}
	if err := r.Update(ctx, r.Route); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to add finalizer: %w", err))
	}
	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) removeRouteFinalizer(ctx context.Context) (*ctrl.Result, error) {
	updated := controllerutil.RemoveFinalizer(r.Route, utils.RouteFinalizer)
	if !updated {
		return subreconciler.ContinueReconciling()
	}
	if err := r.Update(ctx, r.Route); err != nil {
		return subreconciler.RequeueWithError(fmt.Errorf("failed to remove finalizer: %w", err))
	}
	return subreconciler.ContinueReconciling()
}

func (r *RouteReconciler) assembleHttpproxy(ctx context.Context, owner *routev1.Route, sameHostRoutes []routev1.Route) (*contourv1.HTTPProxy, error) {
	httpproxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: consts.HTTPProxyGenerateName,
			Namespace:    owner.Namespace,
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: owner.Spec.Host,
			},
		},
	}

	httpproxy.Spec.IngressClassName = utils.GetIngressClass(owner)

	if owner.Spec.TLS != nil {
		switch owner.Spec.TLS.Termination {
		case "passthrough":
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{
				Passthrough: true,
			}
		case "edge", "reencrypt":
			var secretName string
			if owner.Spec.TLS.Key != "" {
				err := r.ensureTLSSecret(ctx, owner)
				if err != nil {
					return nil, fmt.Errorf("failed to ensure tls secret: %w", err)
				}
				secret, err := r.findTLSSecretByOwner(ctx, owner)
				if err != nil {
					return nil, fmt.Errorf("failed to find tls secret: %w", err)
				}
				secretName = secret.Name
			} else {
				secretName = consts.GlobalTLSSecretName
			}
			httpproxy.Spec.VirtualHost.TLS = &contourv1.TLS{
				SecretName:                secretName,
				EnableFallbackCertificate: true,
			}
		default:
			return nil, fmt.Errorf("invalid termination mode specified on route")
		}
	}

	loadBalancerPolicy, err := utils.GetLoadBalancerPolicy(owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get loadbalancer policy: %w", err)
	}

	// use `tcpproxy` for passthrough mode and `routes` for other termination modes
	if owner.Spec.TLS != nil && owner.Spec.TLS.Termination == "passthrough" {
		httpproxy.Spec.TCPProxy = &contourv1.TCPProxy{}
		for _, sameRoute := range sameHostRoutes {
			ports, err := r.getTargetPorts(ctx, &sameRoute)
			if err != nil {
				return nil, fmt.Errorf("failed to get route target port, %v", err)
			}

			for _, port := range ports {
				svc := contourv1.Service{
					Name:   sameRoute.Spec.To.Name,
					Port:   port,
					Weight: int64(pointer.Int32Deref(sameRoute.Spec.To.Weight, 1)),
				}
				httpproxy.Spec.TCPProxy.Services = append(httpproxy.Spec.TCPProxy.Services, svc)
			}
		}
	} else {
		for _, sameRoute := range sameHostRoutes {
			ports, err := r.getTargetPorts(ctx, &sameRoute)
			if err != nil {
				return nil, fmt.Errorf("failed to get route target port, %v", err)
			}

			for _, port := range ports {
				httpproxyRoute := contourv1.Route{
					Services: []contourv1.Service{
						{
							Name:   sameRoute.Spec.To.Name,
							Port:   port,
							Weight: int64(pointer.Int32Deref(sameRoute.Spec.To.Weight, 1)),
						},
					},
					LoadBalancerPolicy: loadBalancerPolicy,
					TimeoutPolicy: &contourv1.TimeoutPolicy{
						Response: utils.GetTimeout(&sameRoute),
					},
					EnableWebsockets: true,
				}

				rateLimitEnabled, rateLimit := utils.GetRateLimit(&sameRoute)
				if rateLimitEnabled {
					// haproxy router uses 10s window size while we have 1s, 1m, 1h windows in contour
					contourRate := uint32(rateLimit * r.RouterToContourRatio * 6)

					httpproxyRoute.RateLimitPolicy = &contourv1.RateLimitPolicy{
						Local: &contourv1.LocalRateLimitPolicy{
							Requests: contourRate,
							Unit:     consts.RateLimitUnitMinute,
						},
					}
				}

				if sameRoute.Spec.Path != "" {
					httpproxyRoute.Conditions = []contourv1.MatchCondition{
						{Prefix: sameRoute.Spec.Path},
					}
				}

				if sameRoute.Spec.TLS != nil {
					if sameRoute.Spec.TLS.InsecureEdgeTerminationPolicy == routev1.InsecureEdgeTerminationPolicyAllow {
						httpproxyRoute.PermitInsecure = true
					}
				}

				ipWhitelist := utils.GetIPWhitelist(&sameRoute)
				if len(ipWhitelist) > 0 {
					httpproxyRoute.IPAllowFilterPolicy = ipWhitelist
				}

				httpproxy.Spec.Routes = append(httpproxy.Spec.Routes, httpproxyRoute)
			}
		}
	}

	return httpproxy, nil
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
		if port.Protocol != corev1.ProtocolTCP {
			return ports, fmt.Errorf(
				"specified port protocol %s on serice %s is not supported",
				port.Protocol,
				svc.GetName(),
			)
		}
		if port.Name == targetPortName || port.Port == int32(targetPort) {
			ports = []int{int(port.Port)}
			return ports, nil
		}
		ports = append(ports, int(port.Port))
	}

	return ports, nil
}

func (r *RouteReconciler) findTLSSecretByOwner(ctx context.Context, route *routev1.Route) (*corev1.Secret, error) {
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.InNamespace(route.Namespace)); err != nil {
		return nil, err
	}

	for _, secret := range secretList.Items {
		for _, ownerRef := range secret.GetOwnerReferences() {
			if ownerRef.Controller != nil && *ownerRef.Controller && ownerRef.UID == route.UID {
				return &secret, nil
			}
		}
	}
	return nil, consts.NotFoundError
}

func (r *RouteReconciler) ensureTLSSecret(ctx context.Context, route *routev1.Route) error {
	existingSecret, err := r.findTLSSecretByOwner(ctx, route)
	switch {
	case err == nil:
		secret := r.assembleTLSSecret(route)
		if !reflect.DeepEqual(secret.Data, existingSecret.Data) {
			existingSecret.Data = secret.Data
			if err := r.Update(ctx, existingSecret); err != nil {
				return fmt.Errorf("failed to update tls secret: %w", err)
			}
			return nil
		}
		return nil
	case err == consts.NotFoundError:
		secret := r.assembleTLSSecret(route)
		if err := controllerutil.SetControllerReference(route, secret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller owner on secret: %w", err)
		}
		if err := r.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create tls secret: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("failed to find tls secret: %w", err)
	}
}

func (r *RouteReconciler) removeTLSSecret(ctx context.Context) (*ctrl.Result, error) {
	if r.Route.Spec.TLS == nil || r.Route.Spec.TLS.Key == "" {
		return nil, nil
	}

	existingSecret, err := r.findTLSSecretByOwner(ctx, r.Route)
	switch {
	case err == nil:
		return nil, nil
	case err == consts.NotFoundError:
		if err := r.Delete(ctx, existingSecret); err != nil {
			return nil, fmt.Errorf("failed to delete tls secret: %w", err)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("failed to find tls secret: %w", err)
	}
}

func (r *RouteReconciler) assembleTLSSecret(route *routev1.Route) *corev1.Secret {
	encodedKey := make([]byte, base64.StdEncoding.EncodedLen(len(route.Spec.TLS.Key)))
	encodedCrt := make([]byte, base64.StdEncoding.EncodedLen(len(route.Spec.TLS.Certificate)))
	base64.StdEncoding.Encode(encodedKey, []byte(route.Spec.TLS.Key))
	base64.StdEncoding.Encode(encodedCrt, []byte(route.Spec.TLS.Certificate))

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    route.Namespace,
			GenerateName: consts.TLSSecretGenerateName,
		},
		Data: map[string][]byte{
			"tls.key": encodedKey,
			"tls.crt": encodedCrt,
		},
		Type: corev1.SecretTypeTLS,
	}
}

// getSameHostRoutes returns routes with the same host as the given route in the same namespace
// the routes are sorted in ascending order by .metadata.creationTimestamp field
func (r *RouteReconciler) getSameHostRoutes(ctx context.Context, namespace, host string) ([]routev1.Route, error) {
	sameHostRouteList := &routev1.RouteList{}
	if err := r.List(ctx, sameHostRouteList, client.InNamespace(namespace), client.MatchingFields{
		"spec.host": host,
	}); err != nil {
		return nil, err
	}

	sameHostRoutes := make([]routev1.Route, 0, len(sameHostRouteList.Items))
	for _, item := range sameHostRouteList.Items {
		admitted, _ := utils.IsAdmitted(&item)
		if admitted && !utils.IsDeleted(&item) {
			sameHostRoutes = append(sameHostRoutes, item)
		}
	}

	sort.Slice(sameHostRoutes, func(i, j int) bool {
		return sameHostRoutes[i].ObjectMeta.CreationTimestamp.Time.Before(
			sameHostRoutes[j].ObjectMeta.CreationTimestamp.Time,
		)
	})

	return sameHostRoutes, nil
}