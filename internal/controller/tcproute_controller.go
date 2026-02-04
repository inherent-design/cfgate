// Package controller contains the reconciliation logic for cfgate CRDs.
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"cfgate.io/cfgate/internal/controller/features"
)

// TCPRouteReconciler reconciles TCPRoute resources.
//
// SPEC ONLY (alpha.3): This controller is a placeholder for v0.2.0 implementation.
// TCPRoute support requires Cloudflare Spectrum integration which is deferred.
//
// When fully implemented in v0.2.0, this controller will:
// - Validate the cfgate.io/hostname annotation (required for TCPRoute)
// - Validate parent Gateway references
// - Trigger TunnelReconciler to build Spectrum rules
// - Update route status conditions
type TCPRouteReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     events.EventRecorder
	FeatureGates *features.FeatureGates
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/status,verbs=get;update;patch

// Reconcile handles the reconciliation loop for TCPRoute resources.
//
// SPEC ONLY (alpha.3): This method logs that TCPRoute support is spec-only
// and returns early without actual processing. Full implementation in v0.2.0.
func (r *TCPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.V(1).Info("TCPRoute reconciliation skipped: spec-only in alpha.3",
		"name", req.Name,
		"namespace", req.Namespace,
		"implementationVersion", "v0.2.0",
	)

	// Spec-only: no processing, no requeue
	return ctrl.Result{}, nil
}

// SetupWithManager registers the TCPRouteReconciler with the manager.
//
// IMPORTANT: Only call this if FeatureGates.HasTCPRouteSupport() returns true.
// The manager startup in main.go should check feature gates before registration.
//
// Example usage in main.go:
//
//	if featureGates.HasTCPRouteSupport() {
//	    if err = (&controller.TCPRouteReconciler{
//	        Client:       mgr.GetClient(),
//	        Scheme:       mgr.GetScheme(),
//	        Recorder:     mgr.GetEventRecorderFor("tcproute-controller"),
//	        FeatureGates: featureGates,
//	    }).SetupWithManager(mgr); err != nil {
//	        setupLog.Error(err, "unable to create controller", "controller", "TCPRoute")
//	        os.Exit(1)
//	    }
//	    setupLog.Info("TCPRoute controller registered (spec-only in alpha.3)")
//	}
func (r *TCPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Verify feature gate is enabled (defensive check)
	if r.FeatureGates != nil && !r.FeatureGates.HasTCPRouteSupport() {
		// Log and skip registration if CRD not installed
		log := mgr.GetLogger()
		log.V(1).Info("TCPRoute CRD not found, skipping controller registration",
			"requiredVersion", features.V1Alpha2,
			"installHint", "Install Gateway API experimental channel CRDs",
		)
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gwapiv1alpha2.TCPRoute{}).
		Complete(r)
}
