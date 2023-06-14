package utils

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

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

func RemoveOwnerReference(owner, object metav1.Object, scheme *runtime.Scheme) error {
	runtimeObjectOwner, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object", owner)
	}
	if err := validateOwner(owner, object); err != nil {
		return err
	}

	ownerGVK, err := apiutil.GVKForObject(runtimeObjectOwner, scheme)
	if err != nil {
		return err
	}

	ownerRefs := object.GetOwnerReferences()
	modifiedOwnerRefs := make([]metav1.OwnerReference, 0, len(ownerRefs))
	for _, ref := range ownerRefs {
		refGV, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return err
		}
		if ownerGVK.Group != refGV.Group || ownerGVK.Kind != ref.Kind || owner.GetName() != ref.Name {
			modifiedOwnerRefs = append(modifiedOwnerRefs, ref)
		}
	}
	object.SetOwnerReferences(modifiedOwnerRefs)
	return nil
}

// SetControllerReference sets owner as a Controller OwnerReference on controlled.
// copied from controller-runtime/pkg/controller/controllerutil
// The difference between this method and its counterpart in controller runtime is
// this method make the previous contrller owner a normal owner
func SetControllerReference(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object, cannot call SetControllerReference", owner)
	}
	if err := validateOwner(owner, controlled); err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}
	ref := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: pointer.Bool(true),
		Controller:         pointer.Bool(true),
	}

	owners := controlled.GetOwnerReferences()
	newOwners := make([]metav1.OwnerReference, 0, len(owners)+1)
	for _, o := range owners {
		// disable the previous controller
		if o.Controller != nil && *o.Controller {
			o.BlockOwnerDeletion = pointer.Bool(false)
			o.Controller = pointer.Bool(false)
		}
		if referSameObject(o, ref) {
			newOwners = append(newOwners, ref)
		} else {
			newOwners = append(newOwners, o)
		}
	}

	controlled.SetOwnerReferences(newOwners)
	return nil
}

// copied from controller-runtime/pkg/controller/controllerutil
// todo: why not compare UID instead of GV & name?
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV.Group == bGV.Group && a.Kind == b.Kind && a.Name == b.Name
}

// copied from controller-runtime/pkg/controller/controllerutil
func validateOwner(owner, object metav1.Object) error {
	ownerNs := owner.GetNamespace()
	if ownerNs != "" {
		objNs := object.GetNamespace()
		if objNs == "" {
			return fmt.Errorf("cluster-scoped resource must not have a namespace-scoped owner, owner's namespace %s", ownerNs)
		}
		if ownerNs != objNs {
			return fmt.Errorf("cross-namespace owner references are disallowed, owner's namespace %s, obj's namespace %s", owner.GetNamespace(), object.GetNamespace())
		}
	}
	return nil
}
