package consts

type CustomError string

func (e CustomError) Error() string { return string(e) }

const (
	LabelKeyRouterName = "router"

	haproxyAnnotationPrefix = "haproxy.router.openshift.io/"

	AnnotRateLimit         = haproxyAnnotationPrefix + "rate-limit-connections"
	AnnotRateLimitHttpRate = haproxyAnnotationPrefix + "rate-limit-connections.rate-http"
	AnnotBalance           = haproxyAnnotationPrefix + "balance"
	AnnotTimeout           = haproxyAnnotationPrefix + "timeout"
	AnnotIPWhitelist       = haproxyAnnotationPrefix + "ip_whitelist"
	AnnotDisableCookies    = haproxyAnnotationPrefix + "disable_cookies"

	TLSSecretNS         = "openshift-ingress"
	TLSSecretName       = "letsencrypt"
	GlobalTLSSecretName = TLSSecretNS + "/" + TLSSecretName

	RateLimitUnitMinute = "minute"

	RouteShardLabel         = "router"
	DefaultIngressClassName = "private"

	StrategyCookie               = "Cookie"
	StrategyRandom               = "Random"
	StrategyRoundRobin           = "RoundRobin"
	StrategyWeightedLeastRequest = "WeightedLeastRequest"
	StrategyRequestHash          = "RequestHash"
	StrategyDefault              = StrategyWeightedLeastRequest

	HTTPProxyGenerateName = "managed-httpproxy-"
	TLSSecretGenerateName = "managed-tls-secret-"

	NotFoundError = CustomError("not found")
)
