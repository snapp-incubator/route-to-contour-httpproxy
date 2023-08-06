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
	err               error
	ServiceWeight     int32 = 100
	FirstServicePorts       = []v12.ServicePort{{Name: "https", Port: 443}}
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
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout:                      RouteTimeout,
						consts.AnnotationKeyReconciliationPaused: "",
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			fetchRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchRouteFromCluster)).To(Succeed())

			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should exit with no error if the route object is not admitted", func() {
			objRoute := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

			fetchRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchRouteFromCluster)).To(Succeed())

			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(0))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create HTTPProxy object when everything is alright", func() {
			objRoute := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
			Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
		})

		It("should create HTTPProxy object with custom load balancer algorithm (note: this case only tests the algorithm)", func() {
			objRoute := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotBalance:        "roundrobin",
						consts.AnnotDisableCookies: "true",
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].LoadBalancerPolicy.Strategy).To(Equal(consts.StrategyRoundRobin))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create HTTPProxy object with rate limit enabled (note: this case only tests the rate limit)", func() {
			objRoute := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
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
					Host: FirstRouteFQDN,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   FirstServiceName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy.Local.Requests).To(Equal(utils.CalculateRateLimit(cfg.RouterToContourRatio, RateLimitRequests)))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create HTTPProxy object with ip whitelist enabled (note: this case only tests the whitelist)", func() {
			objRoute := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout:     RouteTimeout,
						consts.AnnotIPWhitelist: RouteIPWhiteList,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(1 * time.Second)
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

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create HTTPProxy with TCPProxy when tls is pass through (note: this case only test services on TCPProxy", func() {
			objRoute := routev1.Route{
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationPassthrough,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(BeNil())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(len(httpProxyList.Items[0].Spec.TCPProxy.Services)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.TCPProxy.Services[0].Name).To(Equal(FirstServiceName))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create HTTPProxy with TCPProxy when tls is edge (note: this case only tests tls related configs", func() {
			objRoute := routev1.Route{
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationEdge,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &objRoute)).To(Succeed())

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
			Expect(k8sClient.Status().Update(context.Background(), &objRoute)).To(Succeed())

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.TLS.SecretName).To(Equal(consts.GlobalTLSSecretName))

			Expect(k8sClient.Delete(context.Background(), &objRoute)).To(Succeed())
		})

		It("should create one HTTPProxy object when multiple routes with different hosts exist", func() {
			firstRouteObj := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &firstRouteObj)).To(Succeed())

			secondRouteObj := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      SecondRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: SecondRouteFQDN,
					Path: SecondRoutePath,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   FirstServiceName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &secondRouteObj)).To(Succeed())

			firstRouteObj.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &firstRouteObj)).To(Succeed())

			secondRouteObj.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &secondRouteObj)).To(Succeed())

			fetchFirstRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())

			fetchSecondRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: SecondRouteName}, &fetchSecondRouteFromCluster)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			Expect(k8sClient.Delete(context.Background(), &firstRouteObj)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), &secondRouteObj)).To(Succeed())

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
		})

		It("Should remove the HTTPProxy object and create a new one when the host of the route is changed", func() {
			firstRouteObj := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &firstRouteObj)).To(Succeed())

			firstRouteObj.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &firstRouteObj)).To(Succeed())

			fetchFirstRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			fetchFirstRouteFromCluster = routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster.Spec.Host = FirstRouteUpdatedFQDN
			Expect(k8sClient.Update(context.Background(), &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster = routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &fetchFirstRouteFromCluster)).To(Succeed())

			// wait for new HTTPProxy creation, and deletion of old one
			time.Sleep(2 * time.Second)

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteUpdatedFQDN))

			Expect(k8sClient.Delete(context.Background(), &firstRouteObj)).To(Succeed())

			httpProxyList = contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
		})

		It("should create new HTTPProxy object when there are two routes with same host and the host of older route is changed, also, the older HTTPProxy should change the controller reference to the newer route", func() {
			firstRouteObj := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
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
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			Expect(k8sClient.Create(context.Background(), &firstRouteObj)).To(Succeed())

			secondRouteObj := routev1.Route{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      SecondRouteName,
					Labels: map[string]string{
						consts.RouteShardLabel: RouterName,
					},
					Annotations: map[string]string{
						consts.AnnotTimeout: RouteTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Host: SecondRouteFQDN,
					Path: SecondRoutePath,
					Port: &routev1.RoutePort{
						TargetPort: intstr.IntOrString{IntVal: 443},
					},
					To: routev1.RouteTargetReference{
						Name:   FirstServiceName,
						Kind:   KindService,
						Weight: &ServiceWeight,
					},
					WildcardPolicy: routev1.WildcardPolicyType(FirstRouteWildCardPolicyType),
				},
			}
			// sleep so we can make sure that first route is the older route
			time.Sleep(1 * time.Second)
			Expect(k8sClient.Create(context.Background(), &secondRouteObj)).To(Succeed())

			firstRouteObj.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &firstRouteObj)).To(Succeed())

			secondRouteObj.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &secondRouteObj)).To(Succeed())

			fetchFirstRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())

			fetchSecondRouteFromCluster := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: SecondRouteName}, &fetchSecondRouteFromCluster)).To(Succeed())

			time.Sleep(1 * time.Second)
			httpProxyList := contourv1.HTTPProxyList{}
			Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
			Expect(len(httpProxyList.Items)).To(Equal(1))
			Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))

			// keep track of the HTTPProxy object
			oldHTTPProxyName := httpProxyList.Items[0].Name

			fetchFirstRouteFromCluster = routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster.Spec.Host = FirstRouteUpdatedFQDN
			Expect(k8sClient.Update(context.Background(), &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster = routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &fetchFirstRouteFromCluster)).To(Succeed())
			fetchFirstRouteFromCluster.Status = routev1.RouteStatus{
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
			Expect(k8sClient.Status().Update(context.Background(), &fetchFirstRouteFromCluster)).To(Succeed())

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

			Expect(k8sClient.Delete(context.Background(), &firstRouteObj)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), &secondRouteObj)).To(Succeed())
		})

		It("To enable Debug mode", func() {
			Expect("foo").To(Equal("foo"))
		})
	})
})
