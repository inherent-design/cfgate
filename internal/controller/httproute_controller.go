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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	"cfgate.io/cfgate/internal/controller/annotations"
	"cfgate.io/cfgate/internal/controller/status"
)

// HTTPRouteReconciler reconciles HTTPRoute resources.
// It validates routes against Gateway configuration, resolves backend
// Services, checks annotation validity, and resolves CloudflareAccessPolicy
// references.
type HTTPRouteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflareaccesspolicies,verbs=get;list;watch

// Reconcile handles the reconciliation loop for HTTPRoute resources.
// It validates the route against parent Gateways, validates annotations,
// resolves backend Services, and resolves CloudflareAccessPolicy references.
//
// The reconciliation proceeds through these phases:
//  1. Fetch the HTTPRoute resource
//  2. Validate cfgate.io/* annotations (emit warnings for deprecated ones)
//  3. Preserve other controllers' status.parents[] entries
//  4. Filter and validate only cfgate-managed parentRefs
//  5. Resolve backend Service references
//  6. Resolve cfgate.io/access-policy reference (if present)
//  7. Merge conditions and update route status
//  8. Emit reconciled event
//
// parents[] preservation: Per Gateway API spec, controllers MUST NOT modify
// entries with non-matching controllerName. This implementation preserves
// entries from other controllers (e.g., Istio) and only rebuilds cfgate's
// own entries. Non-cfgate parentRefs are skipped entirely.
//
// On error, the controller requeues after 30 seconds. On success, it requeues
// after 5 minutes for periodic validation.
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("controller").WithName("httproute")
	log.Info("starting reconciliation", "namespace", req.Namespace, "name", req.Name)

	// 1. Fetch HTTPRoute resource
	var route gwapiv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("HTTPRoute not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get HTTPRoute: %w", err)
	}

	// 2. Validate annotations
	validationResult := annotations.ValidateRouteAnnotations(&route, false /* requireHostname */)
	for _, warning := range validationResult.Warnings {
		r.Recorder.Eventf(&route, nil, corev1.EventTypeWarning, "DeprecatedAnnotation", "Validate", warning)
		log.Info("deprecated annotation detected", "warning", warning)
	}
	for _, errMsg := range validationResult.Errors {
		r.Recorder.Eventf(&route, nil, corev1.EventTypeWarning, "InvalidAnnotation", "Validate", errMsg)
		log.Info("invalid annotation value", "error", errMsg)
	}

	// 3. Preserve other controllers' status entries (spec: MUST NOT modify
	// entries with non-matching controllerName). Start from existing parents,
	// remove only cfgate's entries, then rebuild cfgate's entries below.
	var preserved []gwapiv1.RouteParentStatus
	for _, p := range route.Status.Parents {
		if string(p.ControllerName) != GatewayControllerName {
			preserved = append(preserved, p)
		}
	}

	// 4. Validate each parentRef; only process cfgate-managed parents.
	// Per spec: "Implementations of this API can only populate Route status
	// for the Gateways/parent resources they are responsible for."
	var cfgateParentStatuses []gwapiv1.RouteParentStatus
	for _, parentRef := range route.Spec.ParentRefs {
		isCfgate, err := r.isCfgateParentRef(ctx, &route, parentRef)
		if err != nil {
			log.Error(err, "failed to check parentRef ownership")
			continue
		}
		if !isCfgate {
			log.V(1).Info("skipping non-cfgate parentRef",
				"parentRef", parentRef.Name,
			)
			continue
		}
		parentStatus := r.validateParentRef(ctx, &route, parentRef)
		cfgateParentStatuses = append(cfgateParentStatuses, parentStatus)
	}

	// 5. Resolve backend Services
	resolvedRefsCondition := r.resolveBackends(ctx, &route)

	// 6. Resolve access policy reference
	accessPolicyCondition := r.resolveAccessPolicy(ctx, &route)

	// 7. Update route status - merge conditions into each cfgate parent status,
	// then combine with preserved entries from other controllers.
	for i := range cfgateParentStatuses {
		cfgateParentStatuses[i].Conditions = status.MergeConditions(
			cfgateParentStatuses[i].Conditions,
			resolvedRefsCondition,
			accessPolicyCondition,
		)
	}
	route.Status.Parents = append(preserved, cfgateParentStatuses...)

	if err := r.Status().Update(ctx, &route); err != nil {
		log.Error(err, "failed to update route status")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// 8. Emit reconciled event
	r.Recorder.Eventf(&route, nil, corev1.EventTypeNormal, "Reconciled", "Reconcile", "HTTPRoute reconciled successfully")
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
//
// Watched resources:
//   - HTTPRoute (primary resource, with GenerationChangedPredicate)
//   - Gateway (with CfgateAnnotationOrGenerationPredicate for cfgate.io/* annotation awareness)
//   - Service (no predicate -- service changes are rare and important)
//   - CloudflareAccessPolicy (with GenerationChangedPredicate to filter status-only updates)
//
// GenerationChangedPredicate on For() prevents reconciliation on status-only updates,
// reducing spurious reconciliations and status conflicts (137 reconciles/4h + 8 conflicts
// observed without predicate).
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := mgr.GetLogger().WithName("controller").WithName("httproute")
	log.Info("registering controller with manager")
	return ctrl.NewControllerManagedBy(mgr).
		For(&gwapiv1.HTTPRoute{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&gwapiv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findRoutesForGateway),
			builder.WithPredicates(CfgateAnnotationOrGenerationPredicate),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findRoutesForService),
		).
		Watches(
			&cfgatev1alpha1.CloudflareAccessPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.findRoutesForAccessPolicy),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

// findRoutesForGateway returns HTTPRoutes that reference the given Gateway.
func (r *HTTPRouteReconciler) findRoutesForGateway(ctx context.Context, obj client.Object) []reconcile.Request {
	gateway := obj.(*gwapiv1.Gateway)
	log := log.FromContext(ctx)

	var routes gwapiv1.HTTPRouteList
	if err := r.List(ctx, &routes); err != nil {
		log.Error(err, "failed to list HTTPRoutes")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		for _, ref := range route.Spec.ParentRefs {
			refNS := route.Namespace
			if ref.Namespace != nil {
				refNS = string(*ref.Namespace)
			}
			if string(ref.Name) == gateway.Name && refNS == gateway.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      route.Name,
						Namespace: route.Namespace,
					},
				})
				break
			}
		}
	}

	return requests
}

// findRoutesForService returns HTTPRoutes that reference the given Service.
func (r *HTTPRouteReconciler) findRoutesForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc := obj.(*corev1.Service)
	log := log.FromContext(ctx)

	var routes gwapiv1.HTTPRouteList
	if err := r.List(ctx, &routes); err != nil {
		log.Error(err, "failed to list HTTPRoutes")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		for _, rule := range route.Spec.Rules {
			for _, backend := range rule.BackendRefs {
				// Skip non-Service backends
				if backend.Kind != nil && *backend.Kind != "Service" {
					continue
				}
				backendNS := route.Namespace
				if backend.Namespace != nil {
					backendNS = string(*backend.Namespace)
				}
				if string(backend.Name) == svc.Name && backendNS == svc.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      route.Name,
							Namespace: route.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}

// findRoutesForAccessPolicy returns HTTPRoutes that reference the given CloudflareAccessPolicy.
func (r *HTTPRouteReconciler) findRoutesForAccessPolicy(ctx context.Context, obj client.Object) []reconcile.Request {
	policy := obj.(*cfgatev1alpha1.CloudflareAccessPolicy)
	log := log.FromContext(ctx)

	var routes gwapiv1.HTTPRouteList
	if err := r.List(ctx, &routes); err != nil {
		log.Error(err, "failed to list HTTPRoutes")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		policyRef := annotations.GetAnnotation(&route, annotations.AnnotationAccessPolicy)
		if policyRef == "" {
			continue
		}

		// Parse namespace/name format
		policyNS, policyName := parsePolicyRef(policyRef, route.Namespace)
		if policyName == policy.Name && policyNS == policy.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      route.Name,
					Namespace: route.Namespace,
				},
			})
		}
	}

	return requests
}

// isCfgateParentRef checks whether a parentRef points to a Gateway whose
// GatewayClass is managed by cfgate. Returns (true, nil) for cfgate-managed
// parents, (false, nil) for non-cfgate parents or missing resources, and
// (false, err) on unexpected API errors.
func (r *HTTPRouteReconciler) isCfgateParentRef(
	ctx context.Context,
	route *gwapiv1.HTTPRoute,
	ref gwapiv1.ParentReference,
) (bool, error) {
	// Resolve Gateway namespace
	gwNamespace := route.Namespace
	if ref.Namespace != nil {
		gwNamespace = string(*ref.Namespace)
	}

	// Look up the Gateway
	var gateway gwapiv1.Gateway
	if err := r.Get(ctx, types.NamespacedName{
		Name:      string(ref.Name),
		Namespace: gwNamespace,
	}, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			// Gateway not found, cannot determine ownership, skip gracefully
			return false, nil
		}
		return false, fmt.Errorf("failed to get Gateway %s/%s: %w", gwNamespace, ref.Name, err)
	}

	// Look up the GatewayClass
	var gc gwapiv1.GatewayClass
	if err := r.Get(ctx, types.NamespacedName{Name: string(gateway.Spec.GatewayClassName)}, &gc); err != nil {
		if apierrors.IsNotFound(err) {
			// GatewayClass not found, cannot determine ownership, skip gracefully
			return false, nil
		}
		return false, fmt.Errorf("failed to get GatewayClass %s: %w", gateway.Spec.GatewayClassName, err)
	}

	return string(gc.Spec.ControllerName) == GatewayControllerName, nil
}

// validateParentRef validates that the parent Gateway accepts this route.
// Returns a RouteParentStatus with appropriate conditions.
func (r *HTTPRouteReconciler) validateParentRef(
	ctx context.Context,
	route *gwapiv1.HTTPRoute,
	ref gwapiv1.ParentReference,
) gwapiv1.RouteParentStatus {
	log := log.FromContext(ctx)

	// Build base status
	parentNS := gwapiv1.Namespace(route.Namespace)
	if ref.Namespace != nil {
		parentNS = *ref.Namespace
	}

	parentStatus := gwapiv1.RouteParentStatus{
		ParentRef: gwapiv1.ParentReference{
			Group:       ref.Group,
			Kind:        ref.Kind,
			Namespace:   &parentNS,
			Name:        ref.Name,
			SectionName: ref.SectionName,
		},
		ControllerName: GatewayControllerName,
		Conditions: []metav1.Condition{
			status.NewCondition(
				string(gwapiv1.RouteConditionAccepted),
				metav1.ConditionTrue,
				"Accepted",
				"Route accepted by Gateway",
				route.Generation,
			),
		},
	}

	// Get the Gateway
	gwNamespace := route.Namespace
	if ref.Namespace != nil {
		gwNamespace = string(*ref.Namespace)
	}

	var gateway gwapiv1.Gateway
	if err := r.Get(ctx, types.NamespacedName{
		Name:      string(ref.Name),
		Namespace: gwNamespace,
	}, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			parentStatus.Conditions[0] = status.NewCondition(
				string(gwapiv1.RouteConditionAccepted),
				metav1.ConditionFalse,
				"NoMatchingParent",
				fmt.Sprintf("Gateway %s/%s not found", gwNamespace, ref.Name),
				route.Generation,
			)
			return parentStatus
		}
		log.Error(err, "failed to get Gateway")
		return parentStatus
	}

	// Check if Gateway's GatewayClass is ours
	var gc gwapiv1.GatewayClass
	if err := r.Get(ctx, types.NamespacedName{Name: string(gateway.Spec.GatewayClassName)}, &gc); err != nil {
		parentStatus.Conditions[0] = status.NewCondition(
			string(gwapiv1.RouteConditionAccepted),
			metav1.ConditionFalse,
			"NoMatchingParent",
			fmt.Sprintf("GatewayClass %s not found", gateway.Spec.GatewayClassName),
			route.Generation,
		)
		return parentStatus
	}

	if string(gc.Spec.ControllerName) != GatewayControllerName {
		parentStatus.Conditions[0] = status.NewCondition(
			string(gwapiv1.RouteConditionAccepted),
			metav1.ConditionFalse,
			"NoMatchingParent",
			"Gateway is not managed by cfgate",
			route.Generation,
		)
		return parentStatus
	}

	// Check if Gateway has tunnel reference
	if annotations.GetAnnotation(&gateway, annotations.AnnotationTunnelRef) == "" {
		parentStatus.Conditions[0] = status.NewCondition(
			string(gwapiv1.RouteConditionAccepted),
			metav1.ConditionFalse,
			"NoTunnelRef",
			"Gateway has no tunnel reference annotation",
			route.Generation,
		)
		return parentStatus
	}

	// Check listener compatibility if section name specified
	if ref.SectionName != nil {
		found := false
		for _, listener := range gateway.Spec.Listeners {
			if listener.Name == *ref.SectionName {
				found = true
				// Check allowed routes namespace selector
				if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil {
					from := listener.AllowedRoutes.Namespaces.From
					if from != nil && *from == gwapiv1.NamespacesFromSame {
						if route.Namespace != gateway.Namespace {
							parentStatus.Conditions[0] = status.NewCondition(
								string(gwapiv1.RouteConditionAccepted),
								metav1.ConditionFalse,
								"NotAllowedByListeners",
								"Route namespace not allowed by listener",
								route.Generation,
							)
							return parentStatus
						}
					}
				}
				break
			}
		}
		if !found {
			parentStatus.Conditions[0] = status.NewCondition(
				string(gwapiv1.RouteConditionAccepted),
				metav1.ConditionFalse,
				"NoMatchingListenerHostname",
				fmt.Sprintf("Listener %s not found", *ref.SectionName),
				route.Generation,
			)
			return parentStatus
		}
	}

	return parentStatus
}

// resolveBackends resolves backend Service references.
// Returns a ResolvedRefs condition indicating success or failure.
func (r *HTTPRouteReconciler) resolveBackends(
	ctx context.Context,
	route *gwapiv1.HTTPRoute,
) metav1.Condition {
	log := log.FromContext(ctx)

	for _, rule := range route.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			// Skip non-Service backends
			if backend.Kind != nil && *backend.Kind != "Service" {
				continue
			}

			// Get the Service
			namespace := route.Namespace
			if backend.Namespace != nil {
				namespace = string(*backend.Namespace)
			}

			var svc corev1.Service
			if err := r.Get(ctx, types.NamespacedName{
				Name:      string(backend.Name),
				Namespace: namespace,
			}, &svc); err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("backend Service not found",
						"service", backend.Name,
						"namespace", namespace,
					)
					return status.NewCondition(
						string(gwapiv1.RouteConditionResolvedRefs),
						metav1.ConditionFalse,
						"BackendNotFound",
						fmt.Sprintf("Service %s/%s not found", namespace, backend.Name),
						route.Generation,
					)
				}
				log.Error(err, "failed to get Service")
				return status.NewCondition(
					string(gwapiv1.RouteConditionResolvedRefs),
					metav1.ConditionFalse,
					"BackendNotFound",
					fmt.Sprintf("Failed to get Service %s/%s: %v", namespace, backend.Name, err),
					route.Generation,
				)
			}
		}
	}

	return status.NewCondition(
		string(gwapiv1.RouteConditionResolvedRefs),
		metav1.ConditionTrue,
		"ResolvedRefs",
		"All backend references resolved",
		route.Generation,
	)
}

// resolveAccessPolicy resolves the referenced CloudflareAccessPolicy.
// Returns a condition indicating the resolution status.
// If no access-policy annotation is present, returns a success condition.
func (r *HTTPRouteReconciler) resolveAccessPolicy(
	ctx context.Context,
	route *gwapiv1.HTTPRoute,
) metav1.Condition {
	log := log.FromContext(ctx)

	policyRef := annotations.GetAnnotation(route, annotations.AnnotationAccessPolicy)
	if policyRef == "" {
		// No access policy annotation - this is valid
		return status.NewCondition(
			"AccessPolicyResolved",
			metav1.ConditionTrue,
			"NoAccessPolicy",
			"No access policy annotation present",
			route.Generation,
		)
	}

	// Parse namespace/name format
	policyNS, policyName := parsePolicyRef(policyRef, route.Namespace)

	var policy cfgatev1alpha1.CloudflareAccessPolicy
	if err := r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: policyNS,
	}, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("referenced CloudflareAccessPolicy not found",
				"policy", policyRef,
				"parsedNamespace", policyNS,
				"parsedName", policyName,
			)
			r.Recorder.Eventf(route, nil, corev1.EventTypeWarning, "AccessPolicyNotFound", "Resolve",
				"Referenced CloudflareAccessPolicy %q not found", policyRef)
			return status.NewCondition(
				"AccessPolicyResolved",
				metav1.ConditionFalse,
				"AccessPolicyNotFound",
				fmt.Sprintf("CloudflareAccessPolicy %s/%s not found", policyNS, policyName),
				route.Generation,
			)
		}
		log.Error(err, "failed to get CloudflareAccessPolicy")
		return status.NewCondition(
			"AccessPolicyResolved",
			metav1.ConditionFalse,
			"AccessPolicyError",
			fmt.Sprintf("Failed to resolve CloudflareAccessPolicy: %v", err),
			route.Generation,
		)
	}

	log.V(1).Info("resolved access policy",
		"policy", policyRef,
		"applicationId", policy.Status.ApplicationID,
	)
	r.Recorder.Eventf(route, nil, corev1.EventTypeNormal, "AccessPolicyResolved", "Resolve",
		"Attached to CloudflareAccessPolicy %q", policyRef)

	return status.NewCondition(
		"AccessPolicyResolved",
		metav1.ConditionTrue,
		"Resolved",
		fmt.Sprintf("Resolved CloudflareAccessPolicy %s/%s", policyNS, policyName),
		route.Generation,
	)
}

// parsePolicyRef parses a policy reference in "namespace/name" or "name" format.
// Returns (namespace, name). If no namespace specified, uses defaultNS.
func parsePolicyRef(ref string, defaultNS string) (string, string) {
	parts := strings.Split(ref, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return defaultNS, ref
}
