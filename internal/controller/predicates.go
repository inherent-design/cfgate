package controller

import (
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"cfgate.io/cfgate/internal/controller/annotations"
)

// predicateLog is the package-level logger for predicate decisions.
// Uses V(1) per @P-LOG-GO-004 for predicate pass/filter decisions.
var predicateLog = ctrl.Log.WithName("predicate")

// CfgateAnnotationOrGenerationPredicate passes events when:
//  1. metadata.generation changed (spec change), OR
//  2. Any cfgate.io/* annotation changed (value added, removed, or modified)
//
// This replaces GenerationChangedPredicate on cross-resource watchers where
// cfgate annotations (cfgate.io/tunnel-ref, cfgate.io/dns-sync, etc.) are
// meaningful triggers that don't increment generation.
//
// For CRDs with status subresource, annotation-only changes do NOT increment
// metadata.generation. GenerationChangedPredicate alone would filter these events,
// causing the DNS controller to miss annotation additions post-Gateway-creation.
//
// Unlike AnnotationChangedPredicate (which fires on ANY annotation change including
// Helm, Kiali, kubectl metadata), this predicate only checks cfgate.io/* prefixed
// annotations for more precise filtering.
var CfgateAnnotationOrGenerationPredicate = predicate.Or(
	predicate.GenerationChangedPredicate{},
	predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			changed := cfgateAnnotationsChanged(
				e.ObjectOld.GetAnnotations(),
				e.ObjectNew.GetAnnotations(),
			)
			if changed {
				predicateLog.V(1).Info("cfgate annotation change detected",
					"namespace", e.ObjectNew.GetNamespace(),
					"name", e.ObjectNew.GetName(),
					"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
				)
			}
			return changed
		},
	},
)

// GenerationOrDeletionPredicate passes events when:
//  1. metadata.generation changed (spec change), OR
//  2. DeletionTimestamp was just set (object marked for deletion)
//
// This replaces GenerationChangedPredicate on For() clauses for CRDs that use
// finalizers. Without this, setting DeletionTimestamp (which does NOT increment
// generation) produces an Update event that GenerationChangedPredicate filters.
// The reconciler never sees the deletion until a stale RequeueAfter timer fires
// potentially minutes later, causing delayed or failed cleanup.
var GenerationOrDeletionPredicate = predicate.Or(
	predicate.GenerationChangedPredicate{},
	predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			// Pass if DeletionTimestamp was just set.
			if e.ObjectOld.GetDeletionTimestamp() == nil && e.ObjectNew.GetDeletionTimestamp() != nil {
				predicateLog.V(1).Info("deletion timestamp detected",
					"namespace", e.ObjectNew.GetNamespace(),
					"name", e.ObjectNew.GetName(),
				)
				return true
			}
			return false
		},
	},
)

// cfgateAnnotationsChanged returns true if any cfgate.io/* prefixed annotation
// was added, removed, or had its value changed between old and new.
func cfgateAnnotationsChanged(old, new map[string]string) bool {
	// Check for new or changed cfgate annotations
	for k, v := range new {
		if strings.HasPrefix(k, annotations.AnnotationPrefix) {
			if oldV, ok := old[k]; !ok || oldV != v {
				return true // new annotation or changed value
			}
		}
	}
	// Check for removed cfgate annotations
	for k := range old {
		if strings.HasPrefix(k, annotations.AnnotationPrefix) {
			if _, ok := new[k]; !ok {
				return true // annotation removed
			}
		}
	}
	return false
}
