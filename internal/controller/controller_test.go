package controller

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/snapp-incubator/route-to-contour-httpproxy/internal/config"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/utils"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KindService      = "Service"
	DefaultNamespace = "default"
	RouterName       = "default"
	RouteTimeout     = "120s"

	FirstServiceName             = "foo"
	FirstRouteName               = "foo"
	SecondRouteName              = "bar"
	FirstRouteFQDN               = "test.apps.example.com"
	FirstRouteUpdatedFQDN        = "test2.apps.example.com"
	SecondRouteFQDN              = "test.apps.example.com"
	SecondRoutePath              = "/test"
	FirstRouteWildCardPolicyType = "None"

	RateLimitRequests = 100
	RouteIPWhiteList  = "1.1.1.1 8.8.8.8"
)

var (
	ServiceWeight     int32 = 100
	FirstServicePorts       = []v12.ServicePort{{Name: "https", Port: 443}}
)

var _ = Describe("Testing Route to HTTPProxy Controller", func() {
	cfg, errConfig := config.GetConfig("../../hack/config.yaml")
	if errConfig != nil {
		fmt.Println(errConfig, "failed to get config")
		os.Exit(1)
	}

	Context("Testing Reconcile loop functionality", Ordered, func() {
		BeforeAll(func() {
			objService := v12.Service{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstServiceName,
				},
				Spec: v12.ServiceSpec{
					Ports: FirstServicePorts,
					Type:  v12.ServiceTypeClusterIP,
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &objService)).To(Succeed())
		})

		getSampleRoute := func() *routev1.Route {
			return &routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
				},
				Spec: routev1.RouteSpec{
					Host: FirstRouteFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   FirstServiceName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: FirstRouteWildCardPolicyType,
				},
			}
		}

		admitRoute := func(route *routev1.Route) {
			route.Status = routev1.RouteStatus{
				Ingress: []routev1.RouteIngress{
					{
						RouterName: RouterName,
						Conditions: []routev1.RouteIngressCondition{
							{
								Status: v12.ConditionStatus(v1.ConditionTrue),
								Type:   routev1.RouteAdmitted,
							},
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(context.Background(), route)).To(Succeed())
		}

		cleanUpRoute := func(route *routev1.Route) {
			Expect(k8sClient.Delete(context.Background(), route)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: route.Namespace,
					Name:      route.Name,
				}, &routev1.Route{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		}

		It("should exit with no error when the pause label is set, no object should be created", func() {
			route := getSampleRoute()
			route.Annotations = map[string]string{
				consts.AnnotTimeout:                      RouteTimeout,
				consts.AnnotationKeyReconciliationPaused: "",
			}
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			fetchRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchRouteFromCluster)).To(Succeed())

			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			cleanUpRoute(route)
		})

		It("should exit with no error if the route object is not admitted", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			fetchRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchRouteFromCluster)).To(Succeed())

			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object when everything is alright", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
			Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with custom load balancer algorithm (note: this case only tests the algorithm)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotBalance:        "roundrobin",
				consts.AnnotDisableCookies: "true",
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].LoadBalancerPolicy.Strategy).To(Equal(consts.StrategyRoundRobin))

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with rate limit enabled (note: this case only tests the rate limit)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout:           RouteTimeout,
				consts.AnnotRateLimit:         "true",
				consts.AnnotRateLimitHttpRate: strconv.Itoa(RateLimitRequests),
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy.Local.Requests).To(Equal(utils.CalculateRateLimit(cfg.RouterToContourRatio, RateLimitRequests)))

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with ip whitelist enabled (note: this case only tests the whitelist)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout:     RouteTimeout,
				consts.AnnotIPWhitelist: RouteIPWhiteList,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].IPAllowFilterPolicy).NotTo(BeNil())
			for _, ipWhiteList := range strings.Split(RouteIPWhiteList, " ") {
				found := false
				for _, ipFilterPolicy := range httpProxyList.Items[0].Spec.Routes[0].IPAllowFilterPolicy {
					if ipFilterPolicy.CIDR == ipWhiteList {
						found = true
					}
					if found {
						break
					}
				}
				Expect(found).To(BeTrue())
			}

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy with TCPProxy when tls is pass through (note: this case only test services on TCPProxy", func() {
			objRoute := getSampleRoute()
			objRoute.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(BeNil())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(len(httpProxyList.Items[0].Spec.TCPProxy.Services)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.TCPProxy.Services[0].Name).To(Equal(FirstServiceName))

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy with TCPProxy when tls is edge (note: this case only tests tls related configs", func() {
			objRoute := getSampleRoute()
			objRoute.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.TLS.SecretName).To(Equal(consts.GlobalTLSSecretName))

			cleanUpRoute(objRoute)
		})

		It("should create one HTTPProxy object when multiple routes with different hosts exist", func() {
			route1 := getSampleRoute()
			route1.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			Expect(k8sClient.Create(context.Background(), route1)).To(Succeed())

			route2 := getSampleRoute()
			route2.Name = SecondRouteName
			route2.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			route2.Spec.Host = SecondRouteFQDN
			route2.Spec.Path = SecondRoutePath
			Expect(k8sClient.Create(context.Background(), route2)).To(Succeed())

			admitRoute(route1)

			admitRoute(route2)

			fetchFirstRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())

			fetchSecondRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: SecondRouteName}, &fetchSecondRouteFromCluster)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			cleanUpRoute(route1)
			cleanUpRoute(route2)
		})

		It("Should remove the HTTPProxy object and create a new one when the host of the route is changed", func() {
			route := getSampleRoute()
			route.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route)).To(Succeed())

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route)).To(Succeed())
			route.Spec.Host = FirstRouteUpdatedFQDN
			Expect(k8sClient.Update(context.Background(), route)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route)).To(Succeed())
			admitRoute(route)

			// wait for new HTTPProxy creation, and deletion of old one
			time.Sleep(2 * time.Second)

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteUpdatedFQDN))

			cleanUpRoute(route)
		})

		It("should create new HTTPProxy object when there are two routes with same host and the host of older route is changed, also, the older HTTPProxy should change the controller reference to the newer route", func() {
			route1 := getSampleRoute()
			route1.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			Expect(k8sClient.Create(context.Background(), route1)).To(Succeed())

			route2 := getSampleRoute()
			route2.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			route2.Name = SecondRouteName
			route2.Spec.Host = SecondRouteFQDN
			route2.Spec.Path = SecondRoutePath
			// sleep so we can make sure that first route is the older route
			time.Sleep(2 * time.Second)
			Expect(k8sClient.Create(context.Background(), route2)).To(Succeed())

			admitRoute(route1)

			admitRoute(route2)

			time.Sleep(2 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			// keep track of the HTTPProxy object
			oldHTTPProxyName := httpProxyList.Items[0].Name

			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route1)).To(Succeed())
			route1.Spec.Host = FirstRouteUpdatedFQDN
			Expect(k8sClient.Update(context.Background(), route1)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route1)).To(Succeed())
			admitRoute(route1)

			// wait for new HTTPProxy creation, and reconciliation of old one
			time.Sleep(2 * time.Second)

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(2))
			for _, httpProxyObj := range httpProxyList.Items {
				if httpProxyObj.Name == oldHTTPProxyName {
					Expect(httpProxyObj.Spec.VirtualHost.Fqdn).To(Equal(SecondRouteFQDN))
					Expect(len(httpProxyObj.ObjectMeta.OwnerReferences)).To(Equal(1))
					Expect(httpProxyObj.ObjectMeta.OwnerReferences[0].Name).To(Equal(SecondRouteName))
				} else {
					Expect(httpProxyObj.Spec.VirtualHost.Fqdn).To(Equal(FirstRouteUpdatedFQDN))
				}
			}

			cleanUpRoute(route1)
			cleanUpRoute(route2)
		})
	})
})
