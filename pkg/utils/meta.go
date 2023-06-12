package utils

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	AnnotationKeyPrefix               = "snappcloud.io/"
	AnnotationKeyReconciliationPaused = AnnotationKeyPrefix + "paused"

	RouteFinalizer = "snappcloud.io/wait-for-httpproxy-cleanup"
)

func IsDeleted(obj metav1.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}

// IsPaused returns true if the object has the AnnotationKeyReconciliationPaused
func IsPaused(obj metav1.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, ok := annotations[AnnotationKeyReconciliationPaused]
	return ok
}
