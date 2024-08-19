package consts

import (
	"os"
	"strconv"
)

type CustomError string

func (e CustomError) Error() string { return string(e) }

var (
	IngressClassPrivate          = getEnv("INGRESS_CLASS_PRIVATE", "private")
	IngressClassPublic           = getEnv("INGRESS_CLASS_PUBLIC", "public")
	IngressClassInterDc          = getEnv("INGRESS_CLASS_INTER_DC", "inter-dc")
	IngressClassDefault          = getEnv("INGRESS_CLASS_DEFAULT", "private")
	TLSSecretNS                  = getEnv("TLS_SECRET_NS", "openshift-ingress")
	TLSSecretName                = getEnv("TLS_SECRET_NAME", "letsencrypt")
	GlobalTLSSecretName          = TLSSecretNS + "/" + TLSSecretName
	EnableFallbackCertificate, _ = strconv.ParseBool(getEnv("ENABLE_FALLBACK_CERTIFICATE", "true"))
)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

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

	RateLimitUnitMinute = "minute"

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
