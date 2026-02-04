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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	"cfgate.io/cfgate/internal/cloudflare"
	"cfgate.io/cfgate/internal/controller/annotations"
	ctxwrappers "cfgate.io/cfgate/internal/controller/context"
	"cfgate.io/cfgate/internal/controller/features"
	"cfgate.io/cfgate/internal/controller/status"
)

const (
	// accessPolicyFinalizer is the finalizer for CloudflareAccessPolicy resources.
	accessPolicyFinalizer = "cfgate.io/access-policy-cleanup"

	// accessPolicyRequeueAfterError is the requeue delay after an error.
	accessPolicyRequeueAfterError = 30 * time.Second

	// accessPolicyRequeueAfterSuccess is the requeue delay for periodic sync.
	accessPolicyRequeueAfterSuccess = 5 * time.Minute

	// AccessPolicyControllerName is the controller name for policy status.
	AccessPolicyControllerName = "cfgate.io/cloudflare-tunnel-controller"
)

// CloudflareAccessPolicyReconciler reconciles a CloudflareAccessPolicy object.
// It manages the complete Access policy lifecycle: target resolution, application
// creation, policy sync, and optional service tokens and mTLS configuration.
type CloudflareAccessPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder

	// CFClient is the Cloudflare API client. Injected for testing.
	CFClient cloudflare.Client

	// CredentialCache caches validated Cloudflare clients to avoid repeated validations.
	CredentialCache *cloudflare.CredentialCache

	// FeatureGates tracks which optional Gateway API CRDs are available.
	FeatureGates *features.FeatureGates
}

// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflareaccespolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflareaccespolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflareaccespolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflaretunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes;grpcroutes;tcproutes;udproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile handles the reconciliation loop for CloudflareAccessPolicy resources.
// It ensures Access Applications and Policies exist in Cloudflare and are synced.
func (r *CloudflareAccessPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling CloudflareAccessPolicy", "name", req.Name, "namespace", req.Namespace)

	// Phase 1: Fetch CloudflareAccessPolicy resource
	var policy cfgatev1alpha1.CloudflareAccessPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("policy not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get CloudflareAccessPolicy: %w", err)
	}

	// Handle deletion (finalizers)
	if !policy.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &policy)
	}

	// Add finalizer if not present (using patch to reduce lock contention)
	if !controllerutil.ContainsFinalizer(&policy, accessPolicyFinalizer) {
		patch := client.MergeFrom(policy.DeepCopy())
		controllerutil.AddFinalizer(&policy, accessPolicyFinalizer)
		if err := r.Patch(ctx, &policy, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Execute reconciliation phases
	return r.reconcilePhases(ctx, &policy)
}

// reconcilePhases executes the reconciliation phases for CloudflareAccessPolicy.
func (r *CloudflareAccessPolicyReconciler) reconcilePhases(ctx context.Context, policy *cfgatev1alpha1.CloudflareAccessPolicy) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	generation := policy.Generation

	// Phase 2: Resolve credentials
	accessService, accountID, err := r.resolveCredentials(ctx, policy)
	if err != nil {
		log.Error(err, "failed to resolve credentials")
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewCondition(status.ConditionTypeCredentialsValid, metav1.ConditionFalse,
				status.ReasonCredentialsInvalid, status.Error2ConditionMsg(err), generation),
		)
		if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
			log.Error(statusErr, "failed to update status")
		}
		r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "CredentialsInvalid", "Validate", "%s", err.Error())
		return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
	}
	policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
		status.NewCondition(status.ConditionTypeCredentialsValid, metav1.ConditionTrue,
			status.ReasonCredentialsValid, "Credentials validated successfully.", generation),
	)

	// Phase 3: Resolve targets
	policyCtx := ctxwrappers.NewAccessPolicyContext(ctx, policy, r.Client)

	if err := r.validateTargetKinds(ctx, policy); err != nil {
		log.Info("unsupported target kind", "error", err.Error())
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewTargetsResolvedCondition(false, status.ReasonTargetNotFound,
				status.Error2ConditionMsg(err), generation),
		)
		if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
			log.Error(statusErr, "failed to update status")
		}
		return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
	}

	if policyCtx.HasFailedTargets() {
		failedTargets := policyCtx.FailedTargets()
		msg := fmt.Sprintf("Failed to resolve %d target(s): %s", len(failedTargets), failedTargets[0].Error.Error())
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewTargetsResolvedCondition(false, status.ReasonTargetNotFound, msg, generation),
		)
		// Continue with partial resolution if some targets succeeded
		if len(policyCtx.SuccessfullyResolvedTargets()) == 0 {
			if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "TargetNotFound", "Resolve", "%s", msg)
			return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
		}
	} else {
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewTargetsResolvedCondition(true, status.ReasonTargetsResolved,
				fmt.Sprintf("Resolved %d target(s).", len(policyCtx.SuccessfullyResolvedTargets())), generation),
		)
	}

	// Phase 4: Check ReferenceGrants (cross-namespace)
	if policyCtx.HasCrossNamespaceTargets() {
		if err := r.checkReferenceGrants(ctx, policy, policyCtx); err != nil {
			log.Info("ReferenceGrant check failed", "error", err.Error())
			policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
				status.NewCondition("ReferenceGrantValid", metav1.ConditionFalse,
					"ReferenceGrantRequired", status.Error2ConditionMsg(err), generation),
			)
			if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
		}
	}

	// Phase 5: Ensure Access Application
	hostnames, err := policyCtx.ExtractHostnames(ctx, r.Client)
	if err != nil {
		log.Error(err, "failed to extract hostnames")
	}
	app, err := r.ensureApplication(ctx, accessService, accountID, policy, hostnames)
	if err != nil {
		log.Error(err, "failed to ensure Access Application")
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewApplicationCreatedCondition(false, status.ReasonApplicationError,
				status.Error2ConditionMsg(err), generation),
		)
		if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
			log.Error(statusErr, "failed to update status")
		}
		r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "ApplicationError", "Create", "%s", err.Error())
		return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
	}
	policy.Status.ApplicationID = app.ID
	policy.Status.ApplicationAUD = app.AUD
	policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
		status.NewApplicationCreatedCondition(true, status.ReasonApplicationCreated,
			fmt.Sprintf("Access Application %s created.", app.ID), generation),
	)

	// Phase 6: Sync Access Policies
	policyIDs, err := r.syncPolicies(ctx, accessService, accountID, app.ID, policy)
	if err != nil {
		log.Error(err, "failed to sync Access Policies")
		policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
			status.NewPoliciesAttachedCondition(false, status.ReasonPolicyError,
				status.Error2ConditionMsg(err), generation),
		)
		if statusErr := r.updateStatus(ctx, policy); statusErr != nil {
			log.Error(statusErr, "failed to update status")
		}
		r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "PolicySyncError", "Sync", "%s", err.Error())
		return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterError}, nil
	}
	log.V(1).Info("synced access policies", "policyCount", len(policyIDs))
	policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
		status.NewPoliciesAttachedCondition(true, status.ReasonPoliciesAttached,
			fmt.Sprintf("Synced %d Access Policy(ies).", len(policyIDs)), generation),
	)

	// Phase 7: Service Tokens (non-fatal)
	if policyCtx.RequiresServiceTokens() {
		if err := r.ensureServiceTokens(ctx, accessService, accountID, policy); err != nil {
			log.Error(err, "failed to ensure service tokens (continuing)")
			policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
				status.NewCondition("ServiceTokensReady", metav1.ConditionFalse,
					"ServiceTokenError", status.Error2ConditionMsg(err), generation),
			)
		} else {
			policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
				status.NewCondition("ServiceTokensReady", metav1.ConditionTrue,
					"ServiceTokensReady", "Service tokens ready.", generation),
			)
		}
	}

	// Phase 8: mTLS (non-fatal)
	if policyCtx.RequiresMTLS() {
		if err := r.configureMTLS(ctx, accessService, accountID, policy, hostnames); err != nil {
			log.Error(err, "failed to configure mTLS (continuing)")
			policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
				status.NewCondition("MTLSConfigured", metav1.ConditionFalse,
					"MTLSConfigError", status.Error2ConditionMsg(err), generation),
			)
		} else {
			policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions,
				status.NewCondition("MTLSConfigured", metav1.ConditionTrue,
					"MTLSConfigured", "mTLS configured.", generation),
			)
		}
	}

	// Phase 9: Update status
	policy.Status.AttachedTargets = int32(len(policyCtx.SuccessfullyResolvedTargets()))
	policy.Status.ObservedGeneration = generation
	r.updateAncestorStatuses(policy, policyCtx)

	hasServiceTokens := len(policy.Spec.ServiceTokens) > 0
	readyCondition := status.NewAccessPolicyReadyCondition(policy.Status.Conditions, hasServiceTokens, generation)
	policy.Status.Conditions = status.MergeConditions(policy.Status.Conditions, readyCondition)

	if err := r.updateStatus(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(policy, nil, corev1.EventTypeNormal, "Reconciled", "Reconcile",
		"Access policy reconciled successfully")
	return ctrl.Result{RequeueAfter: accessPolicyRequeueAfterSuccess}, nil
}

// resolveCredentials resolves Cloudflare credentials for the policy.
// Uses explicit cloudflareRef or inherits from referenced tunnel.
func (r *CloudflareAccessPolicyReconciler) resolveCredentials(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) (*cloudflare.AccessService, string, error) {
	log := log.FromContext(ctx)

	var secretRef *cfgatev1alpha1.CloudflareSecretRef
	var accountID string

	// Option 1: Explicit cloudflareRef
	if policy.Spec.CloudflareRef != nil {
		secretRef = policy.Spec.CloudflareRef
		accountID = policy.Spec.CloudflareRef.AccountID
		log.V(1).Info("using explicit cloudflareRef",
			"secretName", secretRef.Name,
			"accountId", accountID,
		)
	} else {
		// Option 2: Inherit from tunnelRef (look for cfgate.io/tunnel-ref annotation on targets)
		tunnelCreds, tunnelAccountID, err := r.inheritCredentialsFromTunnel(ctx, policy)
		if err != nil {
			return nil, "", fmt.Errorf("failed to inherit credentials: %w", err)
		}
		if tunnelCreds == nil {
			return nil, "", fmt.Errorf("no credentials configured: set cloudflareRef or ensure targets reference a tunnel")
		}
		secretRef = tunnelCreds
		accountID = tunnelAccountID
		log.V(1).Info("inherited credentials from tunnel",
			"secretName", secretRef.Name,
			"accountId", accountID,
		)
	}

	if accountID == "" {
		return nil, "", fmt.Errorf("account ID not specified and could not be resolved")
	}

	// Create Cloudflare client
	cfClient, err := r.getCloudflareClient(ctx, policy.Namespace, secretRef)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	accessService := cloudflare.NewAccessService(cfClient, log)
	return accessService, accountID, nil
}

// inheritCredentialsFromTunnel finds credentials from Gateway tunnel references.
func (r *CloudflareAccessPolicyReconciler) inheritCredentialsFromTunnel(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) (*cfgatev1alpha1.CloudflareSecretRef, string, error) {
	log := log.FromContext(ctx)

	// Gather all target refs
	refs := policy.Spec.TargetRefs
	if policy.Spec.TargetRef != nil {
		refs = append([]cfgatev1alpha1.PolicyTargetReference{*policy.Spec.TargetRef}, refs...)
	}

	// Find tunnels referenced by target Gateways
	for _, ref := range refs {
		if ref.Kind != "Gateway" {
			continue
		}

		namespace := policy.Namespace
		if ref.Namespace != nil {
			namespace = *ref.Namespace
		}

		var gw gateway.Gateway
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &gw); err != nil {
			continue
		}

		tunnelRef, ok := gw.Annotations[annotations.AnnotationTunnelRef]
		if !ok {
			continue
		}

		// Parse namespace/name
		parts := strings.SplitN(tunnelRef, "/", 2)
		var tunnelNS, tunnelName string
		if len(parts) == 2 {
			tunnelNS, tunnelName = parts[0], parts[1]
		} else {
			tunnelNS, tunnelName = gw.Namespace, parts[0]
		}

		var tunnel cfgatev1alpha1.CloudflareTunnel
		if err := r.Get(ctx, types.NamespacedName{Namespace: tunnelNS, Name: tunnelName}, &tunnel); err != nil {
			log.V(1).Info("failed to get tunnel for credential inheritance",
				"tunnel", tunnelNS+"/"+tunnelName,
				"error", err.Error(),
			)
			continue
		}

		// Found tunnel - inherit credentials
		return &cfgatev1alpha1.CloudflareSecretRef{
			Name:      tunnel.Spec.Cloudflare.SecretRef.Name,
			Namespace: ptr.To(tunnel.Namespace),
			AccountID: tunnel.Spec.Cloudflare.AccountID,
		}, tunnel.Spec.Cloudflare.AccountID, nil
	}

	return nil, "", nil
}

// validateTargetKinds validates that all target kinds are supported.
func (r *CloudflareAccessPolicyReconciler) validateTargetKinds(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) error {
	log := log.FromContext(ctx)

	refs := policy.Spec.TargetRefs
	if policy.Spec.TargetRef != nil {
		refs = append([]cfgatev1alpha1.PolicyTargetReference{*policy.Spec.TargetRef}, refs...)
	}

	for _, ref := range refs {
		switch ref.Kind {
		case "Gateway", "HTTPRoute":
			// Always supported
		case "GRPCRoute":
			if r.FeatureGates != nil && !r.FeatureGates.HasGRPCRouteSupport() {
				log.Info("policy targets GRPCRoute but CRD not installed",
					"policy", policy.Name,
					"targetRef", ref.Name,
				)
				return fmt.Errorf("GRPCRoute CRD not installed")
			}
		case "TCPRoute":
			if r.FeatureGates != nil && !r.FeatureGates.HasTCPRouteSupport() {
				log.Info("policy targets TCPRoute but CRD not installed",
					"policy", policy.Name,
					"targetRef", ref.Name,
				)
				return fmt.Errorf("TCPRoute CRD not installed")
			}
		case "UDPRoute":
			if r.FeatureGates != nil && !r.FeatureGates.HasUDPRouteSupport() {
				log.Info("policy targets UDPRoute but CRD not installed",
					"policy", policy.Name,
					"targetRef", ref.Name,
				)
				return fmt.Errorf("UDPRoute CRD not installed")
			}
		default:
			return fmt.Errorf("unsupported target kind: %s", ref.Kind)
		}
	}
	return nil
}

// checkReferenceGrants verifies cross-namespace references are permitted.
func (r *CloudflareAccessPolicyReconciler) checkReferenceGrants(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
	policyCtx *ctxwrappers.AccessPolicyContext,
) error {
	log := log.FromContext(ctx)

	if r.FeatureGates != nil && !r.FeatureGates.HasReferenceGrantSupport() {
		log.Info("ReferenceGrant CRD not available, cross-namespace references may fail")
		return nil
	}

	for _, target := range policyCtx.ResolvedTargets() {
		if target.Namespace == policy.Namespace {
			continue // Same namespace, no grant needed
		}

		log.V(1).Info("checking ReferenceGrant",
			"fromNamespace", policy.Namespace,
			"toNamespace", target.Namespace,
			"targetKind", target.Kind,
		)

		// List ReferenceGrants in target namespace
		var grants gatewayv1b1.ReferenceGrantList
		if err := r.List(ctx, &grants, client.InNamespace(target.Namespace)); err != nil {
			return fmt.Errorf("listing ReferenceGrants: %w", err)
		}

		permitted := false
		for _, grant := range grants.Items {
			if r.grantPermitsAccess(grant, policy.Namespace, target.Kind) {
				permitted = true
				break
			}
		}

		if !permitted {
			return fmt.Errorf("cross-namespace reference to %s/%s not permitted by ReferenceGrant",
				target.Namespace, target.Name)
		}
	}

	return nil
}

// grantPermitsAccess checks if a ReferenceGrant permits access from the policy namespace.
func (r *CloudflareAccessPolicyReconciler) grantPermitsAccess(
	grant gatewayv1b1.ReferenceGrant,
	fromNamespace, targetKind string,
) bool {
	for _, from := range grant.Spec.From {
		if string(from.Group) != "cfgate.io" {
			continue
		}
		if string(from.Kind) != "CloudflareAccessPolicy" {
			continue
		}
		if string(from.Namespace) != fromNamespace {
			continue
		}

		for _, to := range grant.Spec.To {
			if string(to.Group) == "gateway.networking.k8s.io" &&
				string(to.Kind) == targetKind {
				return true
			}
		}
	}
	return false
}

// ensureApplication ensures the Access Application exists in Cloudflare.
func (r *CloudflareAccessPolicyReconciler) ensureApplication(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID string,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
	hostnames []string,
) (*cloudflare.AccessApplication, error) {
	log := log.FromContext(ctx)

	// Determine domain for application
	domain := policy.Spec.Application.Domain
	if domain == "" && len(hostnames) > 0 {
		domain = hostnames[0] // Use first hostname from targets
		log.V(1).Info("using hostname from target as application domain",
			"domain", domain,
		)
	}
	if domain == "" {
		return nil, fmt.Errorf("no domain configured: set application.domain or target an HTTPRoute with hostnames")
	}

	// Build application params
	params := cloudflare.CreateApplicationParams{
		Name:                    policy.Spec.Application.Name,
		Domain:                  domain,
		Type:                    policy.Spec.Application.Type,
		SessionDuration:         policy.Spec.Application.SessionDuration,
		SkipInterstitial:        policy.Spec.Application.SkipInterstitial,
		EnableBindingCookie:     policy.Spec.Application.EnableBindingCookie,
		SameSiteCookieAttribute: policy.Spec.Application.SameSiteCookieAttribute,
		LogoURL:                 policy.Spec.Application.LogoURL,
		CustomDenyMessage:       policy.Spec.Application.CustomDenyMessage,
		CustomDenyURL:           policy.Spec.Application.CustomDenyURL,
	}

	// Set HttpOnlyCookieAttribute (defaults to true in CRD, need to pass pointer)
	httpOnly := policy.Spec.Application.HttpOnlyCookieAttribute
	params.HttpOnlyCookieAttribute = &httpOnly

	if params.Name == "" {
		params.Name = policy.Name
	}

	log.Info("ensuring Access Application exists",
		"accountId", accountID,
		"domain", domain,
		"applicationName", params.Name,
	)

	app, _, err := accessService.EnsureApplication(ctx, accountID, params)
	return app, err
}

// syncPolicies synchronizes Access Policies for the application.
func (r *CloudflareAccessPolicyReconciler) syncPolicies(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID, appID string,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) ([]string, error) {
	log := log.FromContext(ctx)

	// Convert CRD policies to API params
	var params []cloudflare.CreatePolicyParams
	for i, rule := range policy.Spec.Policies {
		precedence := i + 1
		if rule.Precedence != nil {
			precedence = *rule.Precedence
		}

		params = append(params, cloudflare.CreatePolicyParams{
			Name:                         rule.Name,
			Decision:                     rule.Decision,
			Precedence:                   precedence,
			Include:                      convertAccessRules(rule.Include),
			Exclude:                      convertAccessRules(rule.Exclude),
			Require:                      convertAccessRules(rule.Require),
			SessionDuration:              rule.SessionDuration,
			PurposeJustificationRequired: rule.PurposeJustificationRequired,
			PurposeJustificationPrompt:   rule.PurposeJustificationPrompt,
			ApprovalRequired:             rule.ApprovalRequired,
			ApprovalGroups:               convertApprovalGroups(rule.ApprovalGroups),
		})
	}

	log.Info("syncing access policies",
		"applicationId", appID,
		"policyCount", len(params),
	)

	return accessService.SyncPolicies(ctx, accountID, appID, params)
}

// convertAccessRules converts CRD AccessRule to API AccessRuleParam.
// Implements P0/P1/P2 rule types for alpha.3 (SDK-aligned naming).
func convertAccessRules(crdRules []cfgatev1alpha1.AccessRule) []cloudflare.AccessRuleParam {
	var rules []cloudflare.AccessRuleParam
	for _, r := range crdRules {
		// ============================================================
		// P0: No IdP required
		// ============================================================

		// IP ranges -> multiple rules (SDK: IPRule)
		if r.IP != nil {
			for _, cidr := range r.IP.Ranges {
				cidrCopy := cidr
				rules = append(rules, cloudflare.AccessRuleParam{
					IPRange: &cidrCopy,
				})
			}
			continue
		}

		// IPList -> by ID (SDK: IPListRule)
		if r.IPList != nil && r.IPList.ID != "" {
			id := r.IPList.ID
			rules = append(rules, cloudflare.AccessRuleParam{
				IPListID: &id,
			})
			continue
		}

		// Country codes -> multiple rules (SDK: CountryRule)
		if r.Country != nil {
			for _, code := range r.Country.Codes {
				codeCopy := code
				rules = append(rules, cloudflare.AccessRuleParam{
					Country: &codeCopy,
				})
			}
			continue
		}

		// Everyone (SDK: EveryoneRule)
		if r.Everyone != nil && *r.Everyone {
			everyone := true
			rules = append(rules, cloudflare.AccessRuleParam{
				Everyone: &everyone,
			})
			continue
		}

		// ServiceToken by ID (SDK: ServiceTokenRule)
		if r.ServiceToken != nil && r.ServiceToken.TokenID != "" {
			tokenID := r.ServiceToken.TokenID
			rules = append(rules, cloudflare.AccessRuleParam{
				ServiceTokenID: &tokenID,
			})
			continue
		}

		// AnyValidServiceToken (SDK: AnyValidServiceTokenRule)
		if r.AnyValidServiceToken != nil && *r.AnyValidServiceToken {
			anyValid := true
			rules = append(rules, cloudflare.AccessRuleParam{
				AnyValidServiceToken: &anyValid,
			})
			continue
		}

		// ============================================================
		// P1: Basic IdP required (Google Workspace)
		// ============================================================

		// Email addresses -> multiple rules (SDK: EmailRule)
		if r.Email != nil {
			for _, addr := range r.Email.Addresses {
				addrCopy := addr
				rules = append(rules, cloudflare.AccessRuleParam{
					Email: &addrCopy,
				})
			}
			continue
		}

		// EmailList -> by ID (SDK: EmailListRule)
		if r.EmailList != nil && r.EmailList.ID != "" {
			id := r.EmailList.ID
			rules = append(rules, cloudflare.AccessRuleParam{
				EmailListID: &id,
			})
			continue
		}

		// EmailDomain (SDK: DomainRule)
		if r.EmailDomain != nil {
			domain := r.EmailDomain.Domain
			rules = append(rules, cloudflare.AccessRuleParam{
				EmailDomain: &domain,
			})
			continue
		}

		// OIDCClaim (SDK: AccessOIDCClaimRule)
		if r.OIDCClaim != nil {
			rules = append(rules, cloudflare.AccessRuleParam{
				OIDCClaim: &cloudflare.OIDCClaimParam{
					IdentityProviderID: r.OIDCClaim.IdentityProviderID,
					ClaimName:          r.OIDCClaim.ClaimName,
					ClaimValue:         r.OIDCClaim.ClaimValue,
				},
			})
			continue
		}

		// ============================================================
		// P2: Google Workspace Groups
		// ============================================================

		// GSuiteGroup (SDK: GSuiteGroupRule)
		if r.GSuiteGroup != nil {
			rules = append(rules, cloudflare.AccessRuleParam{
				GSuiteGroup: &cloudflare.GSuiteGroupParam{
					IdentityProviderID: r.GSuiteGroup.IdentityProviderID,
					Email:              r.GSuiteGroup.Email,
				},
			})
			continue
		}

		// ============================================================
		// P3: v0.2.0 - Not implemented in alpha.3
		// ============================================================
		// Certificate, CommonName, Group, GitHub, Azure, Okta, SAML,
		// AuthenticationMethod, DevicePosture, ExternalEvaluation, LoginMethod
	}
	return rules
}

// convertApprovalGroups converts CRD ApprovalGroup to API ApprovalGroupParam.
func convertApprovalGroups(groups []cfgatev1alpha1.ApprovalGroup) []cloudflare.ApprovalGroupParam {
	var result []cloudflare.ApprovalGroupParam
	for _, g := range groups {
		result = append(result, cloudflare.ApprovalGroupParam{
			EmailAddresses:  g.Emails,
			ApprovalsNeeded: g.ApprovalsNeeded,
		})
	}
	return result
}

// ensureServiceTokens ensures service tokens exist and stores credentials in secrets.
func (r *CloudflareAccessPolicyReconciler) ensureServiceTokens(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID string,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) error {
	log := log.FromContext(ctx)

	if policy.Status.ServiceTokenIDs == nil {
		policy.Status.ServiceTokenIDs = make(map[string]string)
	}

	for _, tokenConfig := range policy.Spec.ServiceTokens {
		log.Info("ensuring service token",
			"tokenName", tokenConfig.Name,
			"secretRef", tokenConfig.SecretRef.Name,
		)

		params := cloudflare.CreateServiceTokenParams{
			Name:     tokenConfig.Name,
			Duration: tokenConfig.Duration,
		}

		secretWriter := &k8sSecretWriter{
			client:    r.Client,
			namespace: policy.Namespace,
			secretRef: tokenConfig.SecretRef,
			owner:     policy,
			scheme:    r.Scheme,
		}

		token, err := accessService.EnsureServiceToken(ctx, accountID, params, secretWriter)
		if err != nil {
			return fmt.Errorf("failed to ensure service token %s: %w", tokenConfig.Name, err)
		}

		policy.Status.ServiceTokenIDs[tokenConfig.Name] = token.ID
		log.V(1).Info("service token ready",
			"tokenId", token.ID,
			"tokenName", tokenConfig.Name,
		)
	}

	return nil
}

// k8sSecretWriter implements cloudflare.SecretWriter for Kubernetes secrets.
type k8sSecretWriter struct {
	client    client.Client
	namespace string
	secretRef cfgatev1alpha1.ServiceTokenSecretRef
	owner     *cfgatev1alpha1.CloudflareAccessPolicy
	scheme    *runtime.Scheme
}

// WriteSecret creates or updates a Kubernetes secret with the given data.
func (w *k8sSecretWriter) WriteSecret(ctx context.Context, name string, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.secretRef.Name,
			Namespace: w.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(w.owner, secret, w.scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	// Create or update
	existing := &corev1.Secret{}
	err := w.client.Get(ctx, client.ObjectKeyFromObject(secret), existing)
	if apierrors.IsNotFound(err) {
		return w.client.Create(ctx, secret)
	}
	if err != nil {
		return err
	}

	existing.Data = data
	return w.client.Update(ctx, existing)
}

// configureMTLS configures mTLS certificate and hostname associations.
func (r *CloudflareAccessPolicyReconciler) configureMTLS(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID string,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
	hostnames []string,
) error {
	log := log.FromContext(ctx)

	mtlsConfig := policy.Spec.MTLS
	if mtlsConfig == nil || !mtlsConfig.Enabled {
		return nil
	}

	// Read CA certificate from secret
	if mtlsConfig.RootCASecretRef == nil {
		return fmt.Errorf("mTLS enabled but rootCaSecretRef not specified")
	}

	var caSecret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      mtlsConfig.RootCASecretRef.Name,
	}
	if err := r.Get(ctx, secretKey, &caSecret); err != nil {
		return fmt.Errorf("failed to get CA secret: %w", err)
	}

	key := mtlsConfig.RootCASecretRef.Key
	if key == "" {
		key = "ca.crt"
	}
	caCert, ok := caSecret.Data[key]
	if !ok {
		return fmt.Errorf("CA certificate key %s not found in secret", key)
	}

	// Upload certificate
	ruleName := mtlsConfig.RuleName
	if ruleName == "" {
		ruleName = policy.Name
	}

	log.Info("configuring mTLS certificate",
		"ruleName", ruleName,
		"hostnames", hostnames,
	)

	cert, _, err := accessService.EnsureMTLSCertificate(ctx, accountID, cloudflare.CreateCertificateParams{
		Name:        ruleName,
		Certificate: string(caCert),
	})
	if err != nil {
		return fmt.Errorf("failed to ensure mTLS certificate: %w", err)
	}

	policy.Status.MTLSRuleID = cert.ID

	// Associate hostnames
	associatedHostnames := mtlsConfig.AssociatedHostnames
	if len(associatedHostnames) == 0 {
		associatedHostnames = hostnames
	}

	if len(associatedHostnames) > 0 {
		if err := accessService.UpdateMTLSHostnames(ctx, accountID, associatedHostnames, false); err != nil {
			return fmt.Errorf("failed to update mTLS hostnames: %w", err)
		}
	}

	return nil
}

// reconcileDelete handles policy deletion cleanup.
func (r *CloudflareAccessPolicyReconciler) reconcileDelete(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling deletion of CloudflareAccessPolicy")

	if !controllerutil.ContainsFinalizer(policy, accessPolicyFinalizer) {
		return ctrl.Result{}, nil
	}

	// Check deletion policy annotation
	if policy.Annotations["cfgate.io/deletion-policy"] == "orphan" {
		log.Info("deletion policy is orphan, skipping Cloudflare cleanup")
		return r.removeFinalizer(ctx, policy)
	}

	// Get credentials for deletion
	accessService, accountID, err := r.resolveCredentials(ctx, policy)
	if err != nil {
		log.Error(err, "failed to resolve credentials for deletion, removing finalizer anyway")
		r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "CleanupSkipped", "Delete",
			"Could not resolve credentials for cleanup: %s", err.Error())
		return r.removeFinalizer(ctx, policy)
	}

	// Delete Access Application (cascades to policies)
	if policy.Status.ApplicationID != "" {
		log.Info("deleting Access Application",
			"applicationId", policy.Status.ApplicationID,
		)
		if err := accessService.DeleteApplication(ctx, accountID, policy.Status.ApplicationID); err != nil {
			// Log but don't block finalizer removal
			log.Error(err, "failed to delete Access Application")
			r.Recorder.Eventf(policy, nil, corev1.EventTypeWarning, "CleanupError", "Delete",
				"Failed to delete Access Application: %s", err.Error())
		} else {
			r.Recorder.Eventf(policy, nil, corev1.EventTypeNormal, "ApplicationDeleted", "Delete",
				"Access Application %s deleted", policy.Status.ApplicationID)
		}
	}

	// Revoke service tokens
	for name, tokenID := range policy.Status.ServiceTokenIDs {
		log.V(1).Info("revoking service token",
			"tokenName", name,
			"tokenId", tokenID,
		)
		if err := r.revokeServiceToken(ctx, accessService, accountID, tokenID); err != nil {
			log.Error(err, "failed to revoke service token", "tokenName", name)
		}
	}

	// Remove mTLS rule
	if policy.Status.MTLSRuleID != "" {
		log.V(1).Info("removing mTLS certificate",
			"certificateId", policy.Status.MTLSRuleID,
		)
		if err := r.removeMTLSCertificate(ctx, accessService, accountID, policy.Status.MTLSRuleID); err != nil {
			log.Error(err, "failed to remove mTLS certificate")
		}
	}

	return r.removeFinalizer(ctx, policy)
}

// revokeServiceToken revokes a service token via the API client.
func (r *CloudflareAccessPolicyReconciler) revokeServiceToken(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID, tokenID string,
) error {
	return accessService.Client().DeleteServiceToken(ctx, accountID, tokenID)
}

// removeMTLSCertificate removes an mTLS certificate via the API client.
func (r *CloudflareAccessPolicyReconciler) removeMTLSCertificate(
	ctx context.Context,
	accessService *cloudflare.AccessService,
	accountID, certID string,
) error {
	return accessService.Client().DeleteMTLSCertificate(ctx, accountID, certID)
}

// removeFinalizer removes the finalizer from the policy.
func (r *CloudflareAccessPolicyReconciler) removeFinalizer(
	ctx context.Context,
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
) (ctrl.Result, error) {
	patch := client.MergeFrom(policy.DeepCopy())
	controllerutil.RemoveFinalizer(policy, accessPolicyFinalizer)
	if err := r.Patch(ctx, policy, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

// updateStatus updates the CloudflareAccessPolicy status.
func (r *CloudflareAccessPolicyReconciler) updateStatus(ctx context.Context, policy *cfgatev1alpha1.CloudflareAccessPolicy) error {
	// Re-fetch to avoid conflicts
	var current cfgatev1alpha1.CloudflareAccessPolicy
	if err := r.Get(ctx, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}, &current); err != nil {
		return fmt.Errorf("failed to re-fetch policy: %w", err)
	}

	// Copy status
	current.Status = policy.Status

	if err := r.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// updateAncestorStatuses updates the PolicyAncestorStatus for each target.
func (r *CloudflareAccessPolicyReconciler) updateAncestorStatuses(
	policy *cfgatev1alpha1.CloudflareAccessPolicy,
	policyCtx *ctxwrappers.AccessPolicyContext,
) {
	generation := policy.Generation
	policy.Status.Ancestors = nil // Reset ancestors

	for _, target := range policyCtx.ResolvedTargets() {
		ancestorRef := cfgatev1alpha1.PolicyTargetReference{
			Group:     "gateway.networking.k8s.io",
			Kind:      target.Kind,
			Name:      target.Name,
			Namespace: ptr.To(target.Namespace),
		}

		var conditions []metav1.Condition
		if target.Resolved && target.Error == nil {
			conditions = []metav1.Condition{
				status.NewPolicyAcceptedCondition(true, status.PolicyReasonAccepted,
					"Policy accepted for target.", generation),
			}
		} else {
			msg := "Target not found."
			if target.Error != nil {
				msg = target.Error.Error()
			}
			conditions = []metav1.Condition{
				status.NewPolicyAcceptedCondition(false, status.PolicyReasonTargetNotFound,
					msg, generation),
			}
		}

		policy.Status.Ancestors = append(policy.Status.Ancestors, cfgatev1alpha1.PolicyAncestorStatus{
			AncestorRef:    ancestorRef,
			ControllerName: AccessPolicyControllerName,
			Conditions:     conditions,
		})
	}
}

// getCloudflareClient creates a Cloudflare client from credentials.
func (r *CloudflareAccessPolicyReconciler) getCloudflareClient(
	ctx context.Context,
	policyNamespace string,
	secretRef *cfgatev1alpha1.CloudflareSecretRef,
) (cloudflare.Client, error) {
	// If injected client exists, use it (for testing)
	if r.CFClient != nil {
		return r.CFClient, nil
	}

	// Get credentials from secret
	secretNamespace := policyNamespace
	if secretRef.Namespace != nil && *secretRef.Namespace != "" {
		secretNamespace = *secretRef.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	// Use cache if available
	if r.CredentialCache != nil {
		return r.CredentialCache.GetOrCreate(ctx, secret, func() (cloudflare.Client, error) {
			return r.createClientFromSecret(secret)
		})
	}

	return r.createClientFromSecret(secret)
}

// createClientFromSecret creates a Cloudflare client from a secret.
func (r *CloudflareAccessPolicyReconciler) createClientFromSecret(secret *corev1.Secret) (cloudflare.Client, error) {
	tokenKey := "CLOUDFLARE_API_TOKEN"

	token, ok := secret.Data[tokenKey]
	if !ok {
		return nil, fmt.Errorf("API token key %q not found in secret", tokenKey)
	}

	return cloudflare.NewClient(string(token))
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudflareAccessPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&cfgatev1alpha1.CloudflareAccessPolicy{}).
		Owns(&corev1.Secret{}). // Service token secrets
		Watches(
			&gateway.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForHTTPRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)

	// Conditionally watch GRPCRoute
	if r.FeatureGates != nil && r.FeatureGates.HasGRPCRouteSupport() {
		controllerBuilder = controllerBuilder.Watches(
			&gateway.GRPCRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForGRPCRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)
	}

	// Conditionally watch TCPRoute
	if r.FeatureGates != nil && r.FeatureGates.HasTCPRouteSupport() {
		controllerBuilder = controllerBuilder.Watches(
			&gwapiv1alpha2.TCPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForTCPRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)
	}

	// Conditionally watch UDPRoute
	if r.FeatureGates != nil && r.FeatureGates.HasUDPRouteSupport() {
		controllerBuilder = controllerBuilder.Watches(
			&gwapiv1alpha2.UDPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForUDPRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)
	}

	// Conditionally watch ReferenceGrant
	if r.FeatureGates != nil && r.FeatureGates.HasReferenceGrantSupport() {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1b1.ReferenceGrant{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForReferenceGrant),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		)
	}

	return controllerBuilder.Complete(r)
}

// findPoliciesForHTTPRoute returns reconcile requests for policies targeting this HTTPRoute.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.findPoliciesForTarget(ctx, "HTTPRoute", obj)
}

// findPoliciesForGRPCRoute returns reconcile requests for policies targeting this GRPCRoute.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForGRPCRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.findPoliciesForTarget(ctx, "GRPCRoute", obj)
}

// findPoliciesForTCPRoute returns reconcile requests for policies targeting this TCPRoute.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForTCPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.findPoliciesForTarget(ctx, "TCPRoute", obj)
}

// findPoliciesForUDPRoute returns reconcile requests for policies targeting this UDPRoute.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForUDPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.findPoliciesForTarget(ctx, "UDPRoute", obj)
}

// findPoliciesForTarget finds all policies targeting a specific object.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForTarget(ctx context.Context, kind string, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	// List all CloudflareAccessPolicies
	var policies cfgatev1alpha1.CloudflareAccessPolicyList
	if err := r.List(ctx, &policies); err != nil {
		log.Error(err, "failed to list CloudflareAccessPolicies")
		return nil
	}

	var requests []reconcile.Request
	for _, policy := range policies.Items {
		// Check if policy targets this object
		refs := policy.Spec.TargetRefs
		if policy.Spec.TargetRef != nil {
			refs = append([]cfgatev1alpha1.PolicyTargetReference{*policy.Spec.TargetRef}, refs...)
		}

		for _, ref := range refs {
			if ref.Kind != kind {
				continue
			}
			if ref.Name != obj.GetName() {
				continue
			}

			// Check namespace
			targetNS := policy.Namespace
			if ref.Namespace != nil {
				targetNS = *ref.Namespace
			}
			if targetNS != obj.GetNamespace() {
				continue
			}

			// Match found
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: policy.Namespace,
					Name:      policy.Name,
				},
			})
			break
		}
	}

	if len(requests) > 0 {
		log.V(1).Info("found policies for target",
			"kind", kind,
			"target", obj.GetNamespace()+"/"+obj.GetName(),
			"policyCount", len(requests),
		)
	}

	return requests
}

// findPoliciesForReferenceGrant returns reconcile requests for policies affected by grant changes.
func (r *CloudflareAccessPolicyReconciler) findPoliciesForReferenceGrant(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	grant, ok := obj.(*gatewayv1b1.ReferenceGrant)
	if !ok {
		return nil
	}

	// Find policies in namespaces that reference this grant's namespace
	var policies cfgatev1alpha1.CloudflareAccessPolicyList
	if err := r.List(ctx, &policies); err != nil {
		log.Error(err, "failed to list CloudflareAccessPolicies")
		return nil
	}

	var requests []reconcile.Request
	for _, policy := range policies.Items {
		// Check if policy has cross-namespace refs to grant's namespace
		refs := policy.Spec.TargetRefs
		if policy.Spec.TargetRef != nil {
			refs = append([]cfgatev1alpha1.PolicyTargetReference{*policy.Spec.TargetRef}, refs...)
		}

		for _, ref := range refs {
			if ref.Namespace == nil || *ref.Namespace != grant.Namespace {
				continue
			}
			// This policy references the grant's namespace
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: policy.Namespace,
					Name:      policy.Name,
				},
			})
			break
		}
	}

	return requests
}
