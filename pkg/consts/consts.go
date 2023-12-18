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

	AnnotationKeyPrefix               = "snappcloud.io/"
	AnnotationKeyReconciliationPaused = AnnotationKeyPrefix + "paused"
	AnnotationKeyHttp1Enforced        = AnnotationKeyPrefix + "force-h1"

	TLSSecretNS         = "openshift-ingress"
	TLSSecretName       = "letsencrypt"
	GlobalTLSSecretName = TLSSecretNS + "/" + TLSSecretName

	RateLimitUnitMinute = "minute"

	IngressClassPrivate = "private"
	IngressClassInterDc = "inter-dc"
	IngressClassPublic  = "public"

	RouteShardLabel = "router"

	StrategyCookie               = "Cookie"
	StrategyRandom               = "Random"
	StrategyRoundRobin           = "RoundRobin"
	StrategyWeightedLeastRequest = "WeightedLeastRequest"
	StrategyRequestHash          = "RequestHash"
	StrategyDefault              = StrategyRoundRobin

	TLSSecretGenerateName = "managed-tls-secret-"

	NotFoundError = CustomError("not found")
)
