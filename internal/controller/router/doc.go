package router

// RouteReconciler is responsible for creating proper projectcontour.io/v1/HTTPProxy objects based on the existing
// route.openshift.io/v1/Route objects

// Overall Flow:
// A reconcile request comes to the controller. We'll ignore the request if:
// 		- we can't find a route with the given namespace/name
// 		- the route has paused annotation
// 		- the route doesn't have an admission status

// if the request passes over all the above conditions, we'll decide how to reconcile:
//		- if route has `deletionTimestamp` set or has false admission status:
//			1- try to find the dependeant httpproxy by ownerReferences
//			2- if httpproxy is found:
//				2.1- delete the httpproxy if there's no route with the same host or update it otherwise
//			3- remove the tls secret owned by this route (if exists)
//			3- remove the cleanup finalizer from the route under reconcile
//		- else:
//			1- try to find the dependeant httpproxy by ownerReferences
//			2- if there's such httpproxy and the hosts don't match:
//				2.1- delete or update the httpproxy
//			3- if no httpproxy found in step 1, try to find the httpproxy by matching host
//			4- if an httpproxy found:
//				4.1- update the httpproxy
//			5- else:
//				5.1- create a new httpproxy
//			6- add cleanup finalizer to the route under reconcile

// How we handle 1-n relation between httpproxies and routes:
// Each time a route is reconciled, if it's admitted and not deleted, we'll add it as an ownerReference to an HTTPProxy.
// Also, we set the oldest route as the controller owner (controller=true).
