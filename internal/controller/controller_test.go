package controller

import (
	"github.com/snapp-incubator/route-to-contour-httpproxy/internal/config"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"fmt"
)

const (
	APIVersionRoute   = "route.openshift.io/v1"
	APIVersionService = "v1"
	KindRoute         = "Route"
	KindService       = "Service"
	DefaultNamespace  = "default"
	RouterName        = "default"
	RouteTimeout      = "120s"

	ServiceFooName             = "foo"
	RouteFooName               = "foo"
	RouteBarName               = "bar"
	RouteFooFQDN               = "test.apps.example.com"
	RouteBarFQDN               = "test.apps.example.com"
	RouteBarPath               = "/bar"
	RouteFooWildCardPolicyType = "None"

	RateLimitRequests = 100
	RouteIPWhiteList  = "1.1.1.1 8.8.8.8"
)

var (
	err             error
	ServiceWeight   int32 = 100
	ServiceFooPorts       = []v12.ServicePort{{Name: "https", Port: 443}}
	RouterTypeMeta        = v1.TypeMeta{Kind: KindRoute, APIVersion: APIVersionRoute}
	ServiceTypeMeta       = v1.TypeMeta{Kind: KindService, APIVersion: APIVersionService}
)

var _ = Describe("Testing Route to HTTPProxy Controller", func() {
	cfg, errConfig := config.GetConfig("../../hack/config.yaml")
	if errConfig != nil {
		fmt.Println(err, "failed to get config")
		os.Exit(1)
	}

	Context("Testing Reconcile loop functionality", Serial, func() {
		BeforeEach(func() {
			// Create Service
			objService := v12.Service{
				TypeMeta: ServiceTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      ServiceFooName,
				},
				Spec: v12.ServiceSpec{
					Ports: ServiceFooPorts,
					Type:  v12.ServiceTypeClusterIP,
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			err = k8sClient.Create(context.Background(), &objService)
			if err != nil && errors.IsAlreadyExists(err) {
				fmt.Println(objService.ObjectMeta.Name, "Already exists")
				return
			}
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			// Delete Service
			objService := v12.Service{
				TypeMeta: ServiceTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      ServiceFooName,
				},
				Spec: v12.ServiceSpec{
					Ports: ServiceFooPorts,
					Type:  v12.ServiceTypeClusterIP,
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			err = k8sClient.Delete(context.Background(), &objService)
			if err != nil && errors.IsNotFound(err) {
				fmt.Println(objService.ObjectMeta.Name, "Not found")
				return
			}
			Expect(err).To(BeNil())

			// wait 3s to make sure the route is deleted properly
			time.Sleep(3 * time.Second)

		})

		It("should exit with no error when the pause label is set, no object should be created", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout:                      RouteTimeout,
						consts.AnnotationKeyReconciliationPaused: "",
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			tt := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &tt)
			Expect(err).To(BeNil())

			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should exit with no error if the route object is not admitted", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			tt := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &tt)
			Expect(err).To(BeNil())

			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create HTTPProxy object when everything is alright", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(RouteFooFQDN))
			Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			httpProxyList = contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
		})

		It("should create HTTPProxy object with custom load balancer algorithm (note: this case only tests the algorithm)", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotBalance:        "roundrobin",
						consts.AnnotDisableCookies: "true",
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].LoadBalancerPolicy.Strategy).To(Equal(consts.StrategyRoundRobin))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create HTTPProxy object with rate limit enabled (note: this case only tests the rate limit)", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout:           RouteTimeout,
						consts.AnnotRateLimit:         "true",
						consts.AnnotRateLimitHttpRate: strconv.Itoa(RateLimitRequests),
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy.Local.Requests).To(Equal(utils.CalculateRateLimit(cfg.RouterToContourRatio, RateLimitRequests)))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create HTTPProxy object with ip whitelist enabled (note: this case only tests the whitelist)", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout:     RouteTimeout,
						consts.AnnotIPWhitelist: RouteIPWhiteList,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
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

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create HTTPProxy with TCPProxy when tls is pass through (note: this case only test services on TCPProxy", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationPassthrough,
					},
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(len(httpProxyList.Items[0].Spec.TCPProxy.Services)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.TCPProxy.Services[0].Name).To(Equal(ServiceFooName))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create HTTPProxy with TCPProxy when tls is edge (note: this case only tests tls related configs", func() {
			objRoute := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationEdge,
					},
				},
			}
			err = k8sClient.Create(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			objRoute.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRoute)
			Expect(err).To(BeNil())

			rObj := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObj)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.TLS.SecretName).To(Equal(consts.GlobalTLSSecretName))

			err = k8sClient.Delete(context.Background(), &objRoute)
			Expect(err).To(BeNil())
		})

		It("should create one HTTPProxy object when multiple routes with different hosts exist", func() {
			objRouteFoo := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteFooName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteFooFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRouteFoo)
			Expect(err).To(BeNil())

			objRouteBar := routev1.Route{
				TypeMeta: RouterTypeMeta,
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      RouteBarName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: RouteBarFQDN,
					Path: RouteBarPath,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   ServiceFooName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(RouteFooWildCardPolicyType),
				},
			}
			err = k8sClient.Create(context.Background(), &objRouteBar)
			Expect(err).To(BeNil())

			objRouteFoo.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRouteFoo)
			Expect(err).To(BeNil())

			objRouteBar.Status = routev1.RouteStatus{
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
			err = k8sClient.Status().Update(context.Background(), &objRouteBar)
			Expect(err).To(BeNil())

			rObjFoo := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObjFoo)
			Expect(err).To(BeNil())

			rObjBar := routev1.Route{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: RouteFooName}, &rObjBar)
			Expect(err).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(RouteFooFQDN))

			err = k8sClient.Delete(context.Background(), &objRouteFoo)
			Expect(err).To(BeNil())

			err = k8sClient.Delete(context.Background(), &objRouteBar)
			Expect(err).To(BeNil())

			httpProxyList = contourv1.HTTPProxyList{}
			err = k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))
			Expect(err).To(BeNil())
			Expect(len(httpProxyList.Items)).To(Equal(1))
		})

		It("To enable Debug mode", func() {
			Expect("foo").To(Equal("foo"))
		})
	})
})
