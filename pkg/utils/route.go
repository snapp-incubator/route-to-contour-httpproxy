package utils

import (
	"fmt"
	"strconv"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/snapp-incubator/route-to-contour-httpproxy/internal/config"
	"github.com/snapp-incubator/route-to-contour-httpproxy/pkg/consts"
)

func IsAdmitted(route *routev1.Route) (admitted, hasAdmissionStatus bool) {
	// extract status in the same way as openshift router does
	// .status.ingress[0].conditions[?(@.type=="Admitted")].status
	if len(route.Status.Ingress) < 1 || len(route.Status.Ingress[0].Conditions) < 1 {
		return false, false
	}

	// the default ingress controller selects all routes
	for _, ingress := range route.Status.Ingress {
		if ingress.RouterName == "default" {
			for _, condition := range ingress.Conditions {
				if condition.Type == routev1.RouteAdmitted {
					return condition.Status == v1.ConditionTrue, true
				}
			}
		}
	}

	return false, false
}

func GetIngressClass(route *routev1.Route) string {
	ingressClass := route.Labels[consts.RouteShardLabel]

	switch ingressClass {
	case consts.IngressClassPublic:
		return consts.IngressClassPublic
	case consts.IngressClassInterDc:
		return consts.IngressClassInterDc
	default:
		return consts.IngressClassPrivate
	}
}

func GetIPWhitelist(route *routev1.Route) []contourv1.IPFilterPolicy {
	filterPolicies := make([]contourv1.IPFilterPolicy, 0)
	whitelist, ok := route.Annotations[consts.AnnotIPWhitelist]
	if !ok {
		return filterPolicies
	}
	whitelistCIDRs := strings.Fields(whitelist)
	for _, cidr := range whitelistCIDRs {
		filterPolicies = append(filterPolicies, contourv1.IPFilterPolicy{
			Source: contourv1.IPFilterSourcePeer,
			CIDR:   cidr,
		})
	}
	return filterPolicies
}

func GetLoadBalancerPolicy(route *routev1.Route) (*contourv1.LoadBalancerPolicy, error) {
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

func GetRateLimit(route *routev1.Route) (bool, int) {
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

func GetTimeout(route *routev1.Route, defaultTimeout config.DefaultTimeout) string {
	if timeout, ok := route.Annotations[consts.AnnotTimeout]; ok {
		return timeout
	}

	switch GetIngressClass(route) {
	case consts.IngressClassPublic:
		return defaultTimeout.PublicClass
	case consts.IngressClassInterDc:
		return defaultTimeout.InterDcClass
	default:
		return defaultTimeout.DefaultClass
	}
}

func CalculateRateLimit(ratio, rateLimit int) uint32 {
	return uint32(rateLimit * ratio * 6)
}
