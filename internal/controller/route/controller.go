package route

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger()

	return ctrl.NewControllerManagedBy(mgr).
		For(&routev1.Route{}).
		Owns(&contourv1.HTTPProxy{}).
		Owns(&corev1.Secret{}).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(r.listRoutesForService)).
		Complete(r)
}

// listRoutesForService returns reconcile requests for routes with target service set to obj
func (r *Reconciler) listRoutesForService(ctx context.Context, obj client.Object) (reconcileReqs []reconcile.Request) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	routeList := &routev1.RouteList{}
	if err := r.List(ctx, routeList, client.InNamespace(svc.Namespace)); err != nil {
		r.logger.Error(err, "failed to list routes in watch predicate")
		return
	}

	for _, route := range routeList.Items {
		if route.Spec.To.Name == svc.Name {
			reconcileReqs = append(reconcileReqs, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: route.Namespace, Name: route.Name},
			})
		}
	}
	return
}
