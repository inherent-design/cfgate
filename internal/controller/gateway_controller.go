// Package controller contains the reconciliation logic for cfgate CRDs.
package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	"cfgate.io/cfgate/internal/controller/annotations"
)

const (
	// GatewayControllerName is the controller name for GatewayClass.
	GatewayControllerName = "cfgate.io/cloudflare-tunnel-controller"
)

// GatewayReconciler reconciles Gateway resources that reference CloudflareTunnel.
// It validates tunnel references, updates Gateway status, and triggers
// tunnel configuration syncs when Gateway configuration changes.
type GatewayReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch

// Reconcile handles the reconciliation loop for Gateway resources.
// It validates the tunnel reference and updates Gateway status.
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling Gateway", "name", req.Name, "namespace", req.Namespace)

	// 1. Fetch Gateway resource
	var gateway gwapiv1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Gateway not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Gateway: %w", err)
	}

	// 2. Check if GatewayClass is ours
	isOurs, err := r.isOurGatewayClass(ctx, &gateway)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check GatewayClass: %w", err)
	}
	if !isOurs {
		log.Info("Gateway is not managed by cfgate, ignoring")
		return ctrl.Result{}, nil
	}

	// 3. Validate tunnel reference annotation
	tunnelRef := annotations.GetAnnotation(&gateway, annotations.AnnotationTunnelRef)
	if tunnelRef == "" {
		log.Info("Gateway has no tunnel reference annotation")
		r.setGatewayCondition(&gateway, gwapiv1.GatewayConditionAccepted, metav1.ConditionFalse, "MissingTunnelRef", "cfgate.io/tunnel-ref annotation is required")
		if err := r.Status().Update(ctx, &gateway); err != nil {
			log.Error(err, "failed to update gateway status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// 4. Resolve referenced CloudflareTunnel
	tunnel, err := r.resolveTunnelRef(ctx, &gateway)
	if err != nil {
		log.Error(err, "failed to resolve tunnel reference", "ref", tunnelRef)
		r.setGatewayCondition(&gateway, gwapiv1.GatewayConditionAccepted, metav1.ConditionFalse, "TunnelNotFound", err.Error())
		if err := r.Status().Update(ctx, &gateway); err != nil {
			log.Error(err, "failed to update gateway status")
		}
		r.Recorder.Eventf(&gateway, nil, corev1.EventTypeWarning, "TunnelNotFound", "Validate", "%s", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// 5. Update Gateway status
	if err := r.updateGatewayStatus(ctx, &gateway, tunnel); err != nil {
		log.Error(err, "failed to update gateway status")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	r.Recorder.Eventf(&gateway, nil, corev1.EventTypeNormal, "Reconciled", "Reconcile", "Gateway reconciled successfully")
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gwapiv1.Gateway{}).
		Complete(r)
}

// isOurGatewayClass checks if the GatewayClass is managed by cfgate.
func (r *GatewayReconciler) isOurGatewayClass(ctx context.Context, gateway *gwapiv1.Gateway) (bool, error) {
	var gc gwapiv1.GatewayClass
	if err := r.Get(ctx, types.NamespacedName{Name: string(gateway.Spec.GatewayClassName)}, &gc); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get GatewayClass: %w", err)
	}

	return string(gc.Spec.ControllerName) == GatewayControllerName, nil
}

// resolveTunnelRef resolves the tunnel reference annotation to a CloudflareTunnel.
// Returns the tunnel or an error if not found/invalid.
func (r *GatewayReconciler) resolveTunnelRef(ctx context.Context, gateway *gwapiv1.Gateway) (*cfgatev1alpha1.CloudflareTunnel, error) {
	tunnelRef := annotations.GetAnnotation(gateway, annotations.AnnotationTunnelRef)
	if tunnelRef == "" {
		return nil, fmt.Errorf("missing %s annotation", annotations.AnnotationTunnelRef)
	}

	parts := strings.Split(tunnelRef, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid tunnel reference format: expected 'namespace/name', got %q", tunnelRef)
	}

	namespace := parts[0]
	name := parts[1]

	var tunnel cfgatev1alpha1.CloudflareTunnel
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &tunnel); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("tunnel %s/%s not found", namespace, name)
		}
		return nil, fmt.Errorf("failed to get tunnel: %w", err)
	}

	return &tunnel, nil
}

// updateGatewayStatus updates the Gateway status with tunnel information.
// Sets addresses, conditions, and listener status.
func (r *GatewayReconciler) updateGatewayStatus(ctx context.Context, gateway *gwapiv1.Gateway, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	// Set addresses to tunnel domain
	if tunnel.Status.TunnelDomain != "" {
		gateway.Status.Addresses = []gwapiv1.GatewayStatusAddress{
			{
				Type:  ptrTo(gwapiv1.HostnameAddressType),
				Value: tunnel.Status.TunnelDomain,
			},
		}
	}

	// Set conditions
	if tunnel.Status.TunnelID != "" {
		r.setGatewayCondition(gateway, gwapiv1.GatewayConditionAccepted, metav1.ConditionTrue, "TunnelReady", "Gateway is bound to tunnel")
		r.setGatewayCondition(gateway, gwapiv1.GatewayConditionProgrammed, metav1.ConditionTrue, "Programmed", "Gateway configuration applied")
	} else {
		r.setGatewayCondition(gateway, gwapiv1.GatewayConditionAccepted, metav1.ConditionTrue, "TunnelPending", "Waiting for tunnel to be ready")
		r.setGatewayCondition(gateway, gwapiv1.GatewayConditionProgrammed, metav1.ConditionFalse, "TunnelNotReady", "Tunnel is not ready")
	}

	// Update listener status
	gateway.Status.Listeners = make([]gwapiv1.ListenerStatus, len(gateway.Spec.Listeners))
	for i, listener := range gateway.Spec.Listeners {
		attachedRoutes := r.countAttachedRoutes(ctx, gateway, listener)
		gateway.Status.Listeners[i] = gwapiv1.ListenerStatus{
			Name:           listener.Name,
			AttachedRoutes: attachedRoutes,
			SupportedKinds: []gwapiv1.RouteGroupKind{
				{
					Group: ptrTo(gwapiv1.Group("gateway.networking.k8s.io")),
					Kind:  "HTTPRoute",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:               string(gwapiv1.ListenerConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             "Accepted",
					Message:            "Listener accepted",
					LastTransitionTime: metav1.Now(),
				},
				{
					Type:               string(gwapiv1.ListenerConditionProgrammed),
					Status:             metav1.ConditionTrue,
					Reason:             "Programmed",
					Message:            "Listener programmed",
					LastTransitionTime: metav1.Now(),
				},
			},
		}
	}

	return r.Status().Update(ctx, gateway)
}

// countAttachedRoutes counts the number of routes attached to a Gateway listener.
func (r *GatewayReconciler) countAttachedRoutes(ctx context.Context, gateway *gwapiv1.Gateway, listener gwapiv1.Listener) int32 {
	var routes gwapiv1.HTTPRouteList
	if err := r.List(ctx, &routes); err != nil {
		return 0
	}

	var count int32
	for _, route := range routes.Items {
		for _, parentRef := range route.Spec.ParentRefs {
			parentNS := route.Namespace
			if parentRef.Namespace != nil {
				parentNS = string(*parentRef.Namespace)
			}

			if string(parentRef.Name) == gateway.Name && parentNS == gateway.Namespace {
				// Check section name if specified
				if parentRef.SectionName != nil && *parentRef.SectionName != listener.Name {
					continue
				}
				count++
			}
		}
	}

	return count
}

// setGatewayCondition sets a condition on the Gateway status.
func (r *GatewayReconciler) setGatewayCondition(gateway *gwapiv1.Gateway, conditionType gwapiv1.GatewayConditionType, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: gateway.Generation,
	}

	// Find and update or append
	found := false
	for i, c := range gateway.Status.Conditions {
		if c.Type == string(conditionType) {
			gateway.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		gateway.Status.Conditions = append(gateway.Status.Conditions, condition)
	}
}

// ptrTo returns a pointer to the given value.
func ptrTo[T any](v T) *T {
	return &v
}
