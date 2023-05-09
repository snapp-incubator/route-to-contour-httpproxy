package consts

import "time"

const (
	haproxyAnnotationPrefix = "haproxy.router.openshift.io/"

	AnnotRateLimit         = haproxyAnnotationPrefix + "rate-limit-connections"
	AnnotRateLimitHttpRate = haproxyAnnotationPrefix + "rate-limit-connections.rate-http"
	AnnotBalance           = haproxyAnnotationPrefix + "balance"
	AnnotTimeout           = haproxyAnnotationPrefix + "timeout"
	AnnotIPWhitelist       = haproxyAnnotationPrefix + "ip_whitelist"
	AnnotDisableCookies    = haproxyAnnotationPrefix + "disable_cookies"

	DefaultRequeueTime = 15 * time.Second

	TLSSecretNS   = "openshift-ingress"
	TLSSecretName = "letsencrypt"

	RateLimitUnitMinute   = "minute"
	HAProxyDefaultTimeout = "50s"

	RouteShardLabel         = "router"
	DefaultIngressClassName = "private"
)
