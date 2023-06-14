package route

import (
	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&routev1.Route{}).
		Owns(&contourv1.HTTPProxy{}).
		Complete(r)
}
