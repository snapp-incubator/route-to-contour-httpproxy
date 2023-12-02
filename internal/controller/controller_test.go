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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/snapp-incubator/route-to-contour-httpproxy/internal/config"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/utils"
)

const (
	KindService      = "Service"
	DefaultNamespace = "default"
	RouterName       = "default"
	RouteTimeout     = "120s"

	FirstEndpointsIP             = "10.10.10.10"
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

	WildcardCert = `
-----BEGIN CERTIFICATE-----
MIIDYzCCAksCFCg7O6rz+l2xyXChgE4ae0e7R64MMA0GCSqGSIb3DQEBCwUAMG4x
CzAJBgNVBAYTAklSMQ8wDQYDVQQIDAZUZWhyYW4xDzANBgNVBAcMBlRlaHJhbjEO
MAwGA1UECgwFU25hcHAxEzARBgNVBAsMClNuYXBwQ2xvdWQxGDAWBgNVBAMMDyou
c25hcHBjbG91ZC5pbzAeFw0yMzExMjEyMDE1MzZaFw0yNDExMjAyMDE1MzZaMG4x
CzAJBgNVBAYTAklSMQ8wDQYDVQQIDAZUZWhyYW4xDzANBgNVBAcMBlRlaHJhbjEO
MAwGA1UECgwFU25hcHAxEzARBgNVBAsMClNuYXBwQ2xvdWQxGDAWBgNVBAMMDyou
c25hcHBjbG91ZC5pbzCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALl0
aldira6ue+gD1uJxo2sViJWzmwITtERhX0HMXJRz1/zFZ9dWavbjgPYplUiS1v1g
TymcjDqF2ctChZZAjOs2iGaXixA3lKCbKPzXVAPeqyhTIw0N/rwKbmBGVRhIIwI1
pf1TMyYJiBYuCDN5pf3KZJ1kJ7SqBJO9Qr8wYtZZ+cccvZtpMK+FAsrNef0FFq7P
0ZXpG0/BB5Oyj3OW2jyy1OKx+nfuEKugnQ50SOi2jD1XeSjOK1YysrY2Ucy9QHK7
9p5pQNcy1VyRdqXAlDp2Y3MaoswEIxF6mBrBo6os5JJvXHvGFL5XYOAFTOC+lxW9
SyqpP9DNDnGiQDv5z9ECAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAgjsJTMRGauy7
1BudiattL9C31V5/6tMWf8qATJF7cpdXBS6c5xoMgRg1Uv+E8ZKqH5cjqTbG/Rns
KHUpJngavKMw61yFiyDr6xce2svEfn4+Mr42UpBviVQnfE0cBPd17JHiVMBK2nOG
i7dFAZ0q0nfU3gh4PCGLzdW49tSz3Bt9SDT+9H3t1FnHOdaCcO6SKufnz4IqEoVG
D3ByZu9s4D/4Yoh4NeP/sHEhP3KnTIQQ+4dh1xWs2/5Hd8l5xBid5esWdOrwWb7s
XZyVH1FuumB9pepOLY4TYAskLS2/N47DDFjHRucVxHXkhjASh3RXe7+/YSvCTEwQ
RMsUplRDZg==
-----END CERTIFICATE-----`
	WildcardKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC5dGpXYq2urnvo
A9bicaNrFYiVs5sCE7REYV9BzFyUc9f8xWfXVmr244D2KZVIktb9YE8pnIw6hdnL
QoWWQIzrNohml4sQN5Sgmyj811QD3qsoUyMNDf68Cm5gRlUYSCMCNaX9UzMmCYgW
LggzeaX9ymSdZCe0qgSTvUK/MGLWWfnHHL2baTCvhQLKzXn9BRauz9GV6RtPwQeT
so9zlto8stTisfp37hCroJ0OdEjotow9V3kozitWMrK2NlHMvUByu/aeaUDXMtVc
kXalwJQ6dmNzGqLMBCMRepgawaOqLOSSb1x7xhS+V2DgBUzgvpcVvUsqqT/QzQ5x
okA7+c/RAgMBAAECggEAIaRJICYB7LSxPHLp2bUUmHnVB5cHsPZDFr51Kbn5N2LW
VP+4aRs/lx7JB56eeoZMorUEVz+TPpCGZDVih1GZXpfLYZTvAJecig/rfQZQss0D
TnLaYmVeBt17jVJk4F1BoIZ74HrlxeonuiJKkY/pOSMsYlLHUyIeZ3CHOah83XYw
xstfOpJbSblz7ph4AB27wA6VgSz5xpu7hhUym/cSaDpNO+MpuLc/hvV/Qg4cxfBT
WI64F2vKeeB0adxlN0RC8oX+q1jWHUCGicgwm7Vns3RWgljFQk52QEY4UzwvQD90
z8/RRuWzXoA5lj9Zk6m2Fzm1xatWxSPDAf3/j+lNcwKBgQDm0BtLRGkn9rgdbYiF
lLKuk0F5pfIeEQrETb4jHQURZ9rRQ3+vFEjdKJQkDKLPvdhtUSNAEkyYuzu+Euo8
JpAJn9VOQNgCCe/AynMDKioDzVxS5ZNV8Zf5icsG/697PikmP+xTX+upQ/s+5Ey7
s0WwvX07XNjEwaFTguv4J2aUtwKBgQDNsTOvACzx7VCwFvF/+Xk+vFa+LuWJllIL
HjtL2fvnZwTWNu8TWeQuZKzhlT/jvDdRG9sllh2d5V/3w6I89HN5G7pvRebqtkAl
Q2jO7/cW8s1Mf3YPeHZV5QBhLF9xDjttdWSpKXv9dZwYip45iU5a5UxjwSv/v6Wx
OegWUY+HtwKBgQC/mbN+mKx+O0V9UEbLNLPbTWxF0maZZOY+LJcQyO9DEqZHnrOo
n7sYs629+ytQLjUyEe+kKUyiYJLoZwVAp3ZcNu04B4YIszzuGmC9GMxF2byxJ9hV
uLbCtArwpWGDegdotBm24GJdYYx4GcZE7j2EyNfjZmCffGkyTPUbS4HRIwKBgAPp
IJBtMm2PE3+lkAXc2l9E+Wk4Pwj0oK6xbnMsu8tUfBUOilEV3m67X0YSrlpIE80o
+GuohPuhhseRIp6CD0f4LP08mP1RZbrPo0h763i2OQ0BR19X7PgJGI7AZzghCyQz
nSxSK5dQCx20VPnHEIRN47vpykpcfGv4K99wwYfVAoGBAMt7EHyH9OSUeDxMXYCU
zN9qMOvGvppU7//jgcBe23vFf9s2nv1wjkJCH68Tx/TsMjanCeipcT/weYcZM175
x1FHKiB/ZOqlE9MDamNJlgX+hNJKzJNe9jLkMl1PvUGIwZyV2BrX4VmADLz7jX1Q
VrNBygXnThcxgGU9gP1srPrq
-----END PRIVATE KEY-----`
)

var (
	ServiceWeight int32 = 100
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
					Ports: []v12.ServicePort{
						{Name: "https", Port: 443, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 8443}},
						{Name: "http", Port: 80, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 8080}},
					},
					Type: v12.ServiceTypeClusterIP,
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &objService)).To(Succeed())

			objEndpoints := v12.Endpoints{
				ObjectMeta: v1.ObjectMeta{
					Namespace: DefaultNamespace,
					Name:      FirstServiceName,
				},
				Subsets: []v12.EndpointSubset{
					{
						Addresses: []v12.EndpointAddress{
							{IP: FirstEndpointsIP},
						},
						Ports: []v12.EndpointPort{
							{Name: "https", Port: 8443, Protocol: v12.ProtocolTCP},
							{Name: "http", Port: 8080, Protocol: v12.ProtocolTCP},
						},
					},
				},
			}

			Expect(k8sClient.Create(context.Background(), &objEndpoints)).To(Succeed())
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
						TargetPort: intstr.IntOrString{IntVal: 8443},
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
						RouterName: "default",
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

		It("should create HTTPProxy object when everything is alright (valid targetPort as integer)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			objRoute.Spec.Port.TargetPort = intstr.IntOrString{IntVal: 8443}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))
				g.Expect(len(httpProxyList.Items[0].Spec.Routes[0].Services)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].Services[0].Port).To(Equal(443))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object when everything is alright (valid targetPort as string)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			objRoute.Spec.Port.TargetPort = intstr.IntOrString{Type: intstr.String, StrVal: "https"}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))
				g.Expect(len(httpProxyList.Items[0].Spec.Routes[0].Services)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].Services[0].Port).To(Equal(443))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object when everything is alright (invalid targetPort as integer)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			objRoute.Spec.Port.TargetPort = intstr.IntOrString{IntVal: 443}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))
				g.Expect(len(httpProxyList.Items[0].Spec.Routes)).To(Equal(2))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object when everything is alright (invalid targetPort as string)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout: RouteTimeout,
			}
			objRoute.Spec.Port.TargetPort = intstr.IntOrString{Type: intstr.String, StrVal: "notValid"}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].TimeoutPolicy.Response).To(Equal(RouteTimeout))
				g.Expect(len(httpProxyList.Items[0].Spec.Routes)).To(Equal(2))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with custom load balancer algorithm (only tests the algorithm)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotBalance:        "roundrobin",
				consts.AnnotDisableCookies: "true",
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].LoadBalancerPolicy.Strategy).To(Equal(consts.StrategyRoundRobin))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with rate limit enabled (only tests the rate limit)", func() {
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

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy).NotTo(BeNil())
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy.Local).NotTo(BeNil())
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].RateLimitPolicy.Local.Requests).To(Equal(utils.CalculateRateLimit(cfg.RouterToContourRatio, RateLimitRequests)))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy object with ip whitelist enabled (only tests the whitelist)", func() {
			objRoute := getSampleRoute()
			objRoute.Annotations = map[string]string{
				consts.AnnotTimeout:     RouteTimeout,
				consts.AnnotIPWhitelist: RouteIPWhiteList,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.Routes[0].IPAllowFilterPolicy).NotTo(BeNil())
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
					g.Expect(found).To(BeTrue())
				}
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy with TCPProxy when tls is pass through (only test services on TCPProxy", func() {
			objRoute := getSampleRoute()
			objRoute.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(BeNil())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.TCPProxy).NotTo(BeNil())
				g.Expect(len(httpProxyList.Items[0].Spec.TCPProxy.Services)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.TCPProxy.Services[0].Name).To(Equal(FirstServiceName))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create HTTPProxy with TCPProxy when tls is edge (only tests tls related configs", func() {
			objRoute := getSampleRoute()
			objRoute.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			Expect(k8sClient.Create(context.Background(), objRoute)).To(Succeed())

			admitRoute(objRoute)

			rObj := routev1.Route{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, &rObj)).To(Succeed())

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.TLS.SecretName).To(Equal(consts.GlobalTLSSecretName))
			}).Should(Succeed())

			cleanUpRoute(objRoute)
		})

		It("should create one HTTPProxy object when multiple routes when different hosts exist", func() {
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

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
			}).Should(Succeed())

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

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteFQDN))
			}).Should(Succeed())

			// Update the host
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: DefaultNamespace, Name: FirstRouteName}, route)).To(Succeed())
				route.Spec.Host = FirstRouteUpdatedFQDN
				g.Expect(k8sClient.Update(context.Background(), route)).To(Succeed())
			}).Should(Succeed())
			admitRoute(route)

			// wait for new HTTPProxy creation, and deletion of old one
			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.VirtualHost.Fqdn).To(Equal(FirstRouteUpdatedFQDN))
			}).Should(Succeed())

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

		It("Should set http versions to [http/1.1] for non-inter-dc routes that use the default certificate", func() {
			route := getSampleRoute()
			route.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(len(httpProxyList.Items[0].Spec.HttpVersions)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[0]).To(Equal(contourv1.HttpVersion("http/1.1")))
			}).Should(Succeed())

			cleanUpRoute(route)
		})

		It("Should set http versions to [http/1.1] for non-inter-dc routes that use a custom wildcard certificate", func() {
			route := getSampleRoute()
			route.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			route.Spec.TLS.Key = WildcardKey
			route.Spec.TLS.Certificate = WildcardCert
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(len(httpProxyList.Items[0].Spec.HttpVersions)).To(Equal(1))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[0]).To(Equal(contourv1.HttpVersion("http/1.1")))
			}).Should(Succeed())

			cleanUpRoute(route)
		})

		It("Should set http versions to [h2, http/1.1] for inter-dc routes that use the default certificate", func() {
			route := getSampleRoute()
			route.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			route.Labels[consts.RouteShardLabel] = consts.IngressClassInterDc
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(len(httpProxyList.Items[0].Spec.HttpVersions)).To(Equal(2))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[0]).To(Equal(contourv1.HttpVersion("h2")))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[1]).To(Equal(contourv1.HttpVersion("http/1.1")))
			}).Should(Succeed())

			cleanUpRoute(route)
		})

		It("Should set http versions to [h2, http/1.1] for inter-dc routes that use a custom wildcard certificate", func() {
			route := getSampleRoute()
			route.Spec.TLS = &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			}
			route.Labels[consts.RouteShardLabel] = consts.IngressClassInterDc
			route.Spec.TLS.Key = WildcardKey
			route.Spec.TLS.Certificate = WildcardCert
			Expect(k8sClient.Create(context.Background(), route)).To(Succeed())

			admitRoute(route)

			Eventually(func(g Gomega) {
				httpProxyList := contourv1.HTTPProxyList{}
				g.Expect(k8sClient.List(context.Background(), &httpProxyList, client.InNamespace(DefaultNamespace))).To(Succeed())
				g.Expect(len(httpProxyList.Items)).To(Equal(1))
				g.Expect(len(httpProxyList.Items[0].Spec.HttpVersions)).To(Equal(2))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[0]).To(Equal(contourv1.HttpVersion("h2")))
				g.Expect(httpProxyList.Items[0].Spec.HttpVersions[1]).To(Equal(contourv1.HttpVersion("http/1.1")))
			}).Should(Succeed())

			cleanUpRoute(route)
		})
	})
})
