package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway "sigs.k8s.io/gateway-api/apis/v1"

	cfgatev1alpha1 "cfgate.io/cfgate/api/v1alpha1"
	"cfgate.io/cfgate/internal/cloudflare"
	"cfgate.io/cfgate/internal/cloudflared"
	"cfgate.io/cfgate/internal/controller/annotations"
)

const (
	// tunnelFinalizer is the finalizer for CloudflareTunnel resources.
	tunnelFinalizer = "cfgate.io/tunnel-cleanup"

	// ConditionTypeReady indicates the tunnel is fully operational.
	ConditionTypeReady = "Ready"

	// ConditionTypeCredentialsValid indicates the API credentials are valid.
	ConditionTypeCredentialsValid = "CredentialsValid"

	// ConditionTypeTunnelReady indicates the tunnel exists in Cloudflare.
	ConditionTypeTunnelReady = "TunnelReady"

	// ConditionTypeCloudflaredDeployed indicates the cloudflared deployment is running.
	ConditionTypeCloudflaredDeployed = "CloudflaredDeployed"

	// ConditionTypeConfigurationSynced indicates the tunnel configuration is synced.
	ConditionTypeConfigurationSynced = "ConfigurationSynced"

	// requeueAfterError is the requeue delay after an error.
	requeueAfterError = 30 * time.Second

	// requeueAfterSuccess is the requeue delay for periodic sync.
	requeueAfterSuccess = 5 * time.Minute
)

// CloudflareTunnelReconciler reconciles a CloudflareTunnel object.
// It manages the complete tunnel lifecycle: credential validation, tunnel
// creation/adoption, cloudflared deployment, and configuration sync.
type CloudflareTunnelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder

	// APIReader provides uncached reads for watch mappers to avoid informer lag.
	APIReader client.Reader

	// CFClient is the Cloudflare API client. Injected for testing.
	CFClient cloudflare.Client

	// Builder creates Kubernetes resources for cloudflared.
	Builder cloudflared.Builder

	// CredentialCache caches validated Cloudflare clients to avoid repeated validations.
	CredentialCache *cloudflare.CredentialCache
}

// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflaretunnels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflaretunnels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cfgate.io,resources=cloudflaretunnels/finalizers,verbs=update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the reconciliation loop for CloudflareTunnel resources.
// It ensures the Cloudflare tunnel exists, deploys cloudflared, and syncs configuration.
//
// The reconciliation proceeds through these phases:
//  1. Fetch the CloudflareTunnel resource
//  2. Handle deletion via finalizers (cleanup tunnel from Cloudflare)
//  3. Validate Cloudflare API credentials
//  4. Ensure tunnel exists in Cloudflare (create or adopt)
//  5. Deploy cloudflared connector (Deployment + Secret)
//  6. Sync ingress configuration from Gateway/HTTPRoute resources
//  7. Update status conditions
//
// On error, the controller requeues after 30 seconds. On success, it requeues
// after 5 minutes for periodic configuration sync.
func (r *CloudflareTunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("controller").WithName("tunnel")
	log.Info("starting reconciliation", "namespace", req.Namespace, "name", req.Name)

	// 1. Fetch CloudflareTunnel resource
	var tunnel cfgatev1alpha1.CloudflareTunnel
	if err := r.Get(ctx, req.NamespacedName, &tunnel); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("CloudflareTunnel not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get CloudflareTunnel: %w", err)
	}

	// 2. Handle deletion (finalizers)
	if !tunnel.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &tunnel)
	}

	// Add finalizer if not present (using patch to reduce lock contention)
	if !controllerutil.ContainsFinalizer(&tunnel, tunnelFinalizer) {
		patch := client.MergeFrom(tunnel.DeepCopy())
		controllerutil.AddFinalizer(&tunnel, tunnelFinalizer)
		if err := r.Patch(ctx, &tunnel, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 3. Validate credentials
	if err := r.validateCredentials(ctx, &tunnel); err != nil {
		log.Error(err, "credentials validation failed")
		r.setCondition(&tunnel, ConditionTypeCredentialsValid, metav1.ConditionFalse, "CredentialsInvalid", err.Error())
		r.setCondition(&tunnel, ConditionTypeReady, metav1.ConditionFalse, "CredentialsInvalid", "API credentials are invalid")
		if err := r.updateStatus(ctx, &tunnel); err != nil {
			log.Error(err, "failed to update status")
		}
		r.Recorder.Eventf(&tunnel, nil, corev1.EventTypeWarning, "CredentialsInvalid", "Validate", "%s", err.Error())
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}
	r.setCondition(&tunnel, ConditionTypeCredentialsValid, metav1.ConditionTrue, "CredentialsValid", "API token validated successfully")

	// 4. Resolve/create tunnel
	if err := r.ensureTunnel(ctx, &tunnel); err != nil {
		log.Error(err, "failed to ensure tunnel")
		r.setCondition(&tunnel, ConditionTypeTunnelReady, metav1.ConditionFalse, "TunnelError", err.Error())
		r.setCondition(&tunnel, ConditionTypeReady, metav1.ConditionFalse, "TunnelError", "Failed to ensure tunnel")
		if err := r.updateStatus(ctx, &tunnel); err != nil {
			log.Error(err, "failed to update status")
		}
		r.Recorder.Eventf(&tunnel, nil, corev1.EventTypeWarning, "TunnelError", "Reconcile", "%s", err.Error())
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}
	r.setCondition(&tunnel, ConditionTypeTunnelReady, metav1.ConditionTrue, "TunnelReady", fmt.Sprintf("Tunnel %s ready", tunnel.Status.TunnelID))

	// 5. Deploy cloudflared
	if err := r.deployCloudflared(ctx, &tunnel); err != nil {
		log.Error(err, "failed to deploy cloudflared")
		r.setCondition(&tunnel, ConditionTypeCloudflaredDeployed, metav1.ConditionFalse, "DeploymentError", err.Error())
		r.setCondition(&tunnel, ConditionTypeReady, metav1.ConditionFalse, "DeploymentError", "Failed to deploy cloudflared")
		if err := r.updateStatus(ctx, &tunnel); err != nil {
			log.Error(err, "failed to update status")
		}
		r.Recorder.Eventf(&tunnel, nil, corev1.EventTypeWarning, "DeploymentError", "Deploy", "%s", err.Error())
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}
	r.setCondition(&tunnel, ConditionTypeCloudflaredDeployed, metav1.ConditionTrue, "DeploymentReady", "cloudflared deployment ready")

	// 6. Sync configuration
	if err := r.syncConfiguration(ctx, &tunnel); err != nil {
		log.Error(err, "failed to sync configuration")
		r.setCondition(&tunnel, ConditionTypeConfigurationSynced, metav1.ConditionFalse, "ConfigSyncError", err.Error())
		r.setCondition(&tunnel, ConditionTypeReady, metav1.ConditionFalse, "ConfigSyncError", "Failed to sync configuration")
		if err := r.updateStatus(ctx, &tunnel); err != nil {
			log.Error(err, "failed to update status")
		}
		r.Recorder.Eventf(&tunnel, nil, corev1.EventTypeWarning, "ConfigSyncError", "Sync", "%s", err.Error())
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}
	r.setCondition(&tunnel, ConditionTypeConfigurationSynced, metav1.ConditionTrue, "ConfigurationSynced", fmt.Sprintf("Configuration synced with %d ingress rules", tunnel.Status.ConnectedRouteCount))

	// Note: DNS management is handled by CloudflareDNS CRD

	// 7. Update status
	r.setCondition(&tunnel, ConditionTypeReady, metav1.ConditionTrue, "TunnelOperational", "Tunnel is fully operational")
	tunnel.Status.ObservedGeneration = tunnel.Generation
	now := metav1.Now()
	tunnel.Status.LastSyncTime = &now

	if err := r.updateStatus(ctx, &tunnel); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}

	r.Recorder.Eventf(&tunnel, nil, corev1.EventTypeNormal, "Reconciled", "Reconcile", "Tunnel reconciled successfully")
	return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil
}

// SetupWithManager sets up the controller with the Manager.
// It configures watches for CloudflareTunnel and owned resources.
//
// Watched resources:
//   - CloudflareTunnel (primary resource)
//   - Deployment (owned, for cloudflared)
//   - Secret (owned, for tunnel token)
//   - Gateway (via annotation cfgate.io/tunnel-ref)
//   - HTTPRoute (via parent Gateway reference)
//
// Gateway and HTTPRoute watches use GenerationChangedPredicate to prevent
// reconciliation loops from status-only updates.
func (r *CloudflareTunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := mgr.GetLogger().WithName("controller").WithName("tunnel")
	log.Info("registering controller with manager")
	r.APIReader = mgr.GetAPIReader()

	return ctrl.NewControllerManagedBy(mgr).
		For(&cfgatev1alpha1.CloudflareTunnel{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		// Watch Gateway resources that reference our tunnels
		Watches(
			&gateway.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findTunnelsForGateway),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		// Watch HTTPRoute resources that may affect tunnel configuration
		Watches(
			&gateway.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findTunnelsForHTTPRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

// findTunnelsForGateway returns reconcile requests for tunnels referenced by a Gateway.
func (r *CloudflareTunnelReconciler) findTunnelsForGateway(ctx context.Context, obj client.Object) []reconcile.Request {
	gw, ok := obj.(*gateway.Gateway)
	if !ok {
		return nil
	}

	// Get tunnel reference from annotation
	tunnelRef, ok := gw.Annotations["cfgate.io/tunnel-ref"]
	if !ok {
		return nil
	}

	// Parse namespace/name
	parts := strings.Split(tunnelRef, "/")
	if len(parts) != 2 {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: parts[0],
			Name:      parts[1],
		},
	}}
}

// findTunnelsForHTTPRoute returns reconcile requests for tunnels affected by HTTPRoute changes.
func (r *CloudflareTunnelReconciler) findTunnelsForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gateway.HTTPRoute)
	if !ok {
		return nil
	}

	// Find parent Gateways, then their tunnels
	var requests []reconcile.Request
	for _, parentRef := range route.Spec.ParentRefs {
		// Get the Gateway
		gwNamespace := route.Namespace
		if parentRef.Namespace != nil {
			gwNamespace = string(*parentRef.Namespace)
		}

		gw := &gateway.Gateway{}
		if err := r.APIReader.Get(ctx, types.NamespacedName{
			Namespace: gwNamespace,
			Name:      string(parentRef.Name),
		}, gw); err != nil {
			continue
		}

		// Get tunnel from Gateway annotation
		tunnelRef, ok := gw.Annotations["cfgate.io/tunnel-ref"]
		if !ok {
			continue
		}

		parts := strings.Split(tunnelRef, "/")
		if len(parts) != 2 {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: parts[0],
				Name:      parts[1],
			},
		})
	}

	return requests
}

// validateCredentials validates the Cloudflare API credentials.
// Returns an error if credentials are invalid or missing required permissions.
func (r *CloudflareTunnelReconciler) validateCredentials(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	log := log.FromContext(ctx)

	// Get the Cloudflare client
	cfClient, err := r.getCloudflareClient(ctx, tunnel)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Validate token using operational validation (works for both User and Account tokens)
	accountID, err := r.resolveAccountID(ctx, cfClient, tunnel)
	if err != nil {
		return fmt.Errorf("failed to resolve account: %w", err)
	}
	if err := cfClient.ValidateToken(ctx, accountID); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	log.Info("Cloudflare credentials validated successfully")
	return nil
}

// ensureTunnel ensures the tunnel exists in Cloudflare.
// Creates the tunnel if it doesn't exist, adopts it if it does.
func (r *CloudflareTunnelReconciler) ensureTunnel(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	log := log.FromContext(ctx)

	cfClient, err := r.getCloudflareClient(ctx, tunnel)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	tunnelService := cloudflare.NewTunnelService(cfClient, log)
	accountID, err := r.resolveAccountID(ctx, cfClient, tunnel)
	if err != nil {
		return fmt.Errorf("failed to resolve account: %w", err)
	}

	cfTunnel, created, err := tunnelService.EnsureTunnel(ctx, accountID, tunnel.Spec.Tunnel.Name)
	if err != nil {
		return fmt.Errorf("failed to ensure tunnel: %w", err)
	}

	// Update status with tunnel info
	tunnel.Status.TunnelID = cfTunnel.ID
	tunnel.Status.TunnelName = cfTunnel.Name
	tunnel.Status.TunnelDomain = cloudflare.TunnelDomain(cfTunnel.ID)
	tunnel.Status.AccountID = accountID

	if created {
		log.Info("Created new tunnel", "tunnelID", cfTunnel.ID, "tunnelName", cfTunnel.Name)
		r.Recorder.Eventf(tunnel, nil, corev1.EventTypeNormal, "TunnelCreated", "Create", "Created tunnel %s (ID: %s)", cfTunnel.Name, cfTunnel.ID)
	} else {
		log.Info("Adopted existing tunnel", "tunnelID", cfTunnel.ID, "tunnelName", cfTunnel.Name)
		r.Recorder.Eventf(tunnel, nil, corev1.EventTypeNormal, "TunnelAdopted", "Adopt", "Adopted existing tunnel %s (ID: %s)", cfTunnel.Name, cfTunnel.ID)
	}

	return nil
}

// deployCloudflared ensures the cloudflared Deployment is running.
// Creates or updates the Deployment, ConfigMap, and token Secret.
func (r *CloudflareTunnelReconciler) deployCloudflared(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	log := log.FromContext(ctx)

	if tunnel.Status.TunnelID == "" {
		return fmt.Errorf("tunnel ID not set in status")
	}

	// Get tunnel token
	cfClient, err := r.getCloudflareClient(ctx, tunnel)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	tunnelService := cloudflare.NewTunnelService(cfClient, log)
	accountID, err := r.resolveAccountID(ctx, cfClient, tunnel)
	if err != nil {
		return fmt.Errorf("failed to resolve account: %w", err)
	}

	token, err := tunnelService.GetToken(ctx, accountID, tunnel.Status.TunnelID)
	if err != nil {
		return fmt.Errorf("failed to get tunnel token: %w", err)
	}

	// Get or create builder
	builder := r.Builder
	if builder == nil {
		builder = cloudflared.NewBuilder()
	}

	// Create or update token Secret
	secret := builder.BuildTokenSecret(tunnel, token)
	if err := controllerutil.SetControllerReference(tunnel, secret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set secret owner reference: %w", err)
	}

	existingSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, existingSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, secret); err != nil {
				return fmt.Errorf("failed to create token secret: %w", err)
			}
			log.Info("Created token secret", "name", secret.Name)
		} else {
			return fmt.Errorf("failed to get token secret: %w", err)
		}
	} else {
		existingSecret.Data = secret.Data
		existingSecret.StringData = secret.StringData
		if err := r.Update(ctx, existingSecret); err != nil {
			return fmt.Errorf("failed to update token secret: %w", err)
		}
	}

	// Create or update Deployment
	deployment := builder.BuildDeployment(tunnel, token)
	if err := controllerutil.SetControllerReference(tunnel, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set deployment owner reference: %w", err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			log.Info("Created cloudflared deployment", "name", deployment.Name)
			r.Recorder.Eventf(tunnel, nil, corev1.EventTypeNormal, "DeploymentCreated", "Create", "Created cloudflared deployment %s", deployment.Name)
		} else {
			return fmt.Errorf("failed to get deployment: %w", err)
		}
	} else {
		// Update deployment spec
		existingDeployment.Spec = deployment.Spec
		if err := r.Update(ctx, existingDeployment); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
		log.Info("Updated cloudflared deployment", "name", deployment.Name)
	}

	// Re-fetch deployment to get current status (after create/update)
	if err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment); err != nil {
		log.Error(err, "failed to get deployment status")
	} else {
		tunnel.Status.Replicas = existingDeployment.Status.Replicas
		tunnel.Status.ReadyReplicas = existingDeployment.Status.ReadyReplicas
	}

	return nil
}

// syncConfiguration syncs the tunnel configuration to Cloudflare.
// Collects routes from Gateway/HTTPRoute resources and pushes to Cloudflare API.
func (r *CloudflareTunnelReconciler) syncConfiguration(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	log := log.FromContext(ctx)

	if tunnel.Status.TunnelID == "" {
		return fmt.Errorf("tunnel ID not set in status")
	}

	// Collect ingress rules from HTTPRoutes
	rules, routeCount, err := r.collectIngressRules(ctx, tunnel)
	if err != nil {
		return fmt.Errorf("failed to collect ingress rules: %w", err)
	}

	// Build configuration with defaults
	var defaults *cloudflare.OriginRequestConfig
	if tunnel.Spec.OriginDefaults.ConnectTimeout != "" ||
		tunnel.Spec.OriginDefaults.NoTLSVerify ||
		tunnel.Spec.OriginDefaults.HTTP2Origin {
		defaults = &cloudflare.OriginRequestConfig{
			ConnectTimeout: tunnel.Spec.OriginDefaults.ConnectTimeout,
			NoTLSVerify:    tunnel.Spec.OriginDefaults.NoTLSVerify,
			HTTP2Origin:    tunnel.Spec.OriginDefaults.HTTP2Origin,
		}
	}

	config := cloudflare.BuildConfiguration(rules, defaults)

	// Update fallback target
	if len(config.Ingress) > 0 {
		lastIdx := len(config.Ingress) - 1
		if config.Ingress[lastIdx].Hostname == "" && config.Ingress[lastIdx].Path == "" {
			fallback := tunnel.Spec.FallbackTarget
			if fallback == "" {
				fallback = "http_status:404"
			}
			config.Ingress[lastIdx].Service = fallback
		}
	}

	// Sync to Cloudflare
	cfClient, err := r.getCloudflareClient(ctx, tunnel)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	tunnelService := cloudflare.NewTunnelService(cfClient, log)
	accountID, err := r.resolveAccountID(ctx, cfClient, tunnel)
	if err != nil {
		return fmt.Errorf("failed to resolve account: %w", err)
	}

	if err := tunnelService.UpdateConfiguration(ctx, accountID, tunnel.Status.TunnelID, config); err != nil {
		// Check for 404 - tunnel not found on Cloudflare side
		// This can happen if the tunnel was deleted externally; clear status to force re-adoption
		errStr := err.Error()
		if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "Tunnel not found") {
			log.Info("Tunnel not found on Cloudflare, clearing tunnelID to force re-adoption", "tunnelID", tunnel.Status.TunnelID)
			tunnel.Status.TunnelID = ""
			tunnel.Status.TunnelName = ""
			tunnel.Status.TunnelDomain = ""
			if statusErr := r.Status().Update(ctx, tunnel); statusErr != nil {
				log.Error(statusErr, "failed to clear stale tunnelID from status")
			}
			// Return the original error to trigger requeue, which will re-adopt/create the tunnel
		}
		return fmt.Errorf("failed to update tunnel configuration: %w", err)
	}

	tunnel.Status.ConnectedRouteCount = int32(routeCount)
	log.Info("Synced tunnel configuration", "rules", len(config.Ingress), "routes", routeCount)

	return nil
}

// collectIngressRules collects ingress rules from HTTPRoutes that reference this tunnel.
func (r *CloudflareTunnelReconciler) collectIngressRules(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) ([]cloudflare.IngressRule, int, error) {
	var rules []cloudflare.IngressRule
	routeCount := 0

	// Find Gateways that reference this tunnel
	var gateways gateway.GatewayList
	if err := r.List(ctx, &gateways); err != nil {
		return nil, 0, fmt.Errorf("failed to list gateways: %w", err)
	}

	tunnelRef := fmt.Sprintf("%s/%s", tunnel.Namespace, tunnel.Name)
	var relevantGateways []gateway.Gateway

	for _, gw := range gateways.Items {
		if ref, ok := gw.Annotations[annotations.AnnotationTunnelRef]; ok && ref == tunnelRef {
			relevantGateways = append(relevantGateways, gw)
		}
	}

	// For each Gateway, find HTTPRoutes
	for _, gw := range relevantGateways {
		var routes gateway.HTTPRouteList
		if err := r.List(ctx, &routes); err != nil {
			return nil, 0, fmt.Errorf("failed to list httproutes: %w", err)
		}

		for _, route := range routes.Items {
			// Check if route references this gateway
			for _, parentRef := range route.Spec.ParentRefs {
				parentNS := route.Namespace
				if parentRef.Namespace != nil {
					parentNS = string(*parentRef.Namespace)
				}

				if string(parentRef.Name) == gw.Name && parentNS == gw.Namespace {
					// This route belongs to our gateway
					routeRules := r.buildRulesFromHTTPRoute(&route)
					rules = append(rules, routeRules...)
					routeCount++
				}
			}
		}
	}

	return rules, routeCount, nil
}

// buildRulesFromHTTPRoute builds ingress rules from an HTTPRoute.
func (r *CloudflareTunnelReconciler) buildRulesFromHTTPRoute(route *gateway.HTTPRoute) []cloudflare.IngressRule {
	var rules []cloudflare.IngressRule

	for _, hostname := range route.Spec.Hostnames {
		for _, rule := range route.Spec.Rules {
			// Get path from matches
			path := ""
			if len(rule.Matches) > 0 && rule.Matches[0].Path != nil {
				if rule.Matches[0].Path.Value != nil {
					path = *rule.Matches[0].Path.Value
				}
			}

			// Get backend service
			if len(rule.BackendRefs) > 0 {
				backend := rule.BackendRefs[0]
				servicePort := int32(80)
				if backend.Port != nil {
					servicePort = int32(*backend.Port)
				}

				service := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
					backend.Name, route.Namespace, servicePort)

				ingressRule := cloudflare.IngressRule{
					Hostname: string(hostname),
					Path:     path,
					Service:  service,
				}

				// Apply origin config from annotations
				originConfig := cloudflared.BuildOriginConfig(nil, route.Annotations)
				if originConfig != nil {
					ingressRule.OriginRequest = &cloudflare.OriginRequestConfig{
						ConnectTimeout: originConfig.ConnectTimeout,
						NoTLSVerify:    originConfig.NoTLSVerify,
						HTTP2Origin:    originConfig.HTTP2Origin,
						HTTPHostHeader: originConfig.HTTPHostHeader,
					}
				}

				rules = append(rules, ingressRule)
			}
		}
	}

	return rules
}

// reconcileDelete handles CloudflareTunnel deletion by deleting tunnel connections,
// the tunnel itself (unless orphan policy set), and removing the finalizer.
// Uses fallback credentials if primary credentials are unavailable.
func (r *CloudflareTunnelReconciler) reconcileDelete(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("handling tunnel deletion", "name", tunnel.Name)

	if !controllerutil.ContainsFinalizer(tunnel, tunnelFinalizer) {
		return ctrl.Result{}, nil
	}

	// Note: DNS cleanup is handled by CloudflareDNS CRD reconciler

	// Check deletion policy
	deletionPolicy := tunnel.Annotations["cfgate.io/deletion-policy"]
	if deletionPolicy == "orphan" {
		log.Info("Orphaning tunnel due to deletion policy", "tunnelID", tunnel.Status.TunnelID)
		r.Recorder.Eventf(tunnel, nil, corev1.EventTypeNormal, "TunnelOrphaned", "Delete", "Tunnel %s orphaned due to deletion policy", tunnel.Status.TunnelID)
	} else if tunnel.Status.TunnelID != "" {
		// Try to delete tunnel from Cloudflare
		cfClient, err := r.getCloudflareClientForDeletion(ctx, tunnel)
		if err != nil {
			// Could not get credentials from either primary or fallback
			log.Error(err, "failed to create Cloudflare client for deletion, tunnel may be orphaned on Cloudflare")
			r.Recorder.Eventf(tunnel, nil, corev1.EventTypeWarning, "TunnelOrphanedNoCredentials", "Delete",
				"Tunnel %s may be orphaned on Cloudflare: %v", tunnel.Status.TunnelID, err)
			// Continue with finalizer removal - don't block deletion
		} else {
			tunnelService := cloudflare.NewTunnelService(cfClient, log)
			accountID, err := r.resolveAccountID(ctx, cfClient, tunnel)
			if err != nil {
				log.Error(err, "failed to resolve account for deletion, using cached accountID")
				accountID = tunnel.Status.AccountID // Use cached value from status
			}

			if accountID == "" {
				log.Error(nil, "no account ID available for deletion, tunnel may be orphaned")
				r.Recorder.Eventf(tunnel, nil, corev1.EventTypeWarning, "TunnelOrphanedNoAccountID", "Delete",
					"Tunnel %s may be orphaned on Cloudflare: no account ID", tunnel.Status.TunnelID)
			} else if err := tunnelService.Delete(ctx, accountID, tunnel.Status.TunnelID); err != nil {
				log.Error(err, "failed to delete tunnel from Cloudflare")
				r.Recorder.Eventf(tunnel, nil, corev1.EventTypeWarning, "TunnelDeleteError", "Delete", "%s", err.Error())
				// Continue with finalizer removal
			} else {
				log.Info("Deleted tunnel from Cloudflare", "tunnelID", tunnel.Status.TunnelID)
				r.Recorder.Eventf(tunnel, nil, corev1.EventTypeNormal, "TunnelDeleted", "Delete", "Deleted tunnel %s from Cloudflare", tunnel.Status.TunnelID)
			}
		}
	}

	// Remove finalizer using patch to reduce lock contention
	patch := client.MergeFrom(tunnel.DeepCopy())
	controllerutil.RemoveFinalizer(tunnel, tunnelFinalizer)
	if err := r.Patch(ctx, tunnel, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the CloudflareTunnel status, re-fetching the resource
// first to avoid update conflicts from concurrent modifications.
func (r *CloudflareTunnelReconciler) updateStatus(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) error {
	// Re-fetch to avoid conflicts
	var current cfgatev1alpha1.CloudflareTunnel
	if err := r.Get(ctx, types.NamespacedName{Name: tunnel.Name, Namespace: tunnel.Namespace}, &current); err != nil {
		return fmt.Errorf("failed to re-fetch tunnel: %w", err)
	}

	if tunnelStatusEqual(&current.Status, &tunnel.Status) {
		return nil
	}

	current.Status = tunnel.Status

	if err := r.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// tunnelStatusEqual compares two CloudflareTunnel statuses for equality, ignoring
// LastSyncTime which changes on every reconciliation to avoid spurious updates.
func tunnelStatusEqual(a, b *cfgatev1alpha1.CloudflareTunnelStatus) bool {
	// Compare generation
	if a.ObservedGeneration != b.ObservedGeneration {
		return false
	}

	// Compare string fields
	if a.TunnelID != b.TunnelID ||
		a.TunnelName != b.TunnelName ||
		a.TunnelDomain != b.TunnelDomain ||
		a.AccountID != b.AccountID {
		return false
	}

	// Compare int32 fields
	if a.Replicas != b.Replicas ||
		a.ReadyReplicas != b.ReadyReplicas ||
		a.ConnectedRouteCount != b.ConnectedRouteCount {
		return false
	}

	// Compare conditions (ignoring LastTransitionTime)
	if len(a.Conditions) != len(b.Conditions) {
		return false
	}
	for i := range a.Conditions {
		if a.Conditions[i].Type != b.Conditions[i].Type ||
			a.Conditions[i].Status != b.Conditions[i].Status ||
			a.Conditions[i].Reason != b.Conditions[i].Reason ||
			a.Conditions[i].Message != b.Conditions[i].Message {
			return false
		}
	}

	return true
}

// getCloudflareClient returns a Cloudflare client for the tunnel, creating one
// if needed. Uses credential cache to avoid repeated API validations.
func (r *CloudflareTunnelReconciler) getCloudflareClient(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) (cloudflare.Client, error) {
	// If injected client exists, use it (for testing)
	if r.CFClient != nil {
		return r.CFClient, nil
	}

	// Get credentials from secret
	secretNamespace := tunnel.Spec.Cloudflare.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = tunnel.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      tunnel.Spec.Cloudflare.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	// Use cache if available
	if r.CredentialCache != nil {
		return r.CredentialCache.GetOrCreate(ctx, secret, func() (cloudflare.Client, error) {
			return r.createClientFromSecret(secret, tunnel.Spec.Cloudflare.SecretKeys.APIToken)
		})
	}

	return r.createClientFromSecret(secret, tunnel.Spec.Cloudflare.SecretKeys.APIToken)
}

// createClientFromSecret creates a Cloudflare client from a secret.
func (r *CloudflareTunnelReconciler) createClientFromSecret(secret *corev1.Secret, tokenKey string) (cloudflare.Client, error) {
	if tokenKey == "" {
		tokenKey = "CLOUDFLARE_API_TOKEN"
	}

	token, ok := secret.Data[tokenKey]
	if !ok {
		return nil, fmt.Errorf("API token key %q not found in secret", tokenKey)
	}

	return cloudflare.NewClient(string(token))
}

// getCloudflareClientForDeletion returns a Cloudflare client for tunnel deletion,
// trying primary credentials first then fallback if the primary secret was deleted.
func (r *CloudflareTunnelReconciler) getCloudflareClientForDeletion(ctx context.Context, tunnel *cfgatev1alpha1.CloudflareTunnel) (cloudflare.Client, error) {
	log := log.FromContext(ctx)

	// Try primary credentials first
	cfClient, err := r.getCloudflareClient(ctx, tunnel)
	if err == nil {
		return cfClient, nil
	}

	// Check if we have fallback credentials
	if tunnel.Spec.FallbackCredentialsRef == nil {
		return nil, fmt.Errorf("primary credentials unavailable and no fallback configured: %w", err)
	}

	log.Info("using fallback credentials for deletion",
		"fallbackSecret", tunnel.Spec.FallbackCredentialsRef.Name,
		"fallbackNamespace", tunnel.Spec.FallbackCredentialsRef.Namespace)

	// Try fallback credentials
	fallbackNamespace := tunnel.Spec.FallbackCredentialsRef.Namespace
	if fallbackNamespace == "" {
		fallbackNamespace = tunnel.Namespace
	}

	fallbackSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      tunnel.Spec.FallbackCredentialsRef.Name,
		Namespace: fallbackNamespace,
	}, fallbackSecret); err != nil {
		return nil, fmt.Errorf("failed to get fallback credentials secret: %w", err)
	}

	// Use same token key as primary
	tokenKey := tunnel.Spec.Cloudflare.SecretKeys.APIToken
	if tokenKey == "" {
		tokenKey = "CLOUDFLARE_API_TOKEN"
	}

	token, ok := fallbackSecret.Data[tokenKey]
	if !ok {
		return nil, fmt.Errorf("API token key %q not found in fallback secret", tokenKey)
	}

	return cloudflare.NewClient(string(token))
}

// resolveAccountID returns the Cloudflare account ID with priority:
// spec.cloudflare.accountId > status.accountId (cached) > resolve from accountName via API.
func (r *CloudflareTunnelReconciler) resolveAccountID(ctx context.Context, cfClient cloudflare.Client, tunnel *cfgatev1alpha1.CloudflareTunnel) (string, error) {
	// If accountId is explicitly set in spec, use it
	if tunnel.Spec.Cloudflare.AccountID != "" {
		return tunnel.Spec.Cloudflare.AccountID, nil
	}

	// If we already resolved it, return cached value from status
	if tunnel.Status.AccountID != "" {
		return tunnel.Status.AccountID, nil
	}

	// Resolve accountName to accountId via API
	if tunnel.Spec.Cloudflare.AccountName != "" {
		account, err := cfClient.GetAccountByName(ctx, tunnel.Spec.Cloudflare.AccountName)
		if err != nil {
			return "", fmt.Errorf("failed to resolve account name %q: %w", tunnel.Spec.Cloudflare.AccountName, err)
		}
		if account == nil {
			return "", fmt.Errorf("account %q not found", tunnel.Spec.Cloudflare.AccountName)
		}
		return account.ID, nil
	}

	return "", fmt.Errorf("neither accountId nor accountName specified")
}

// setCondition sets a condition on the tunnel status.
func (r *CloudflareTunnelReconciler) setCondition(tunnel *cfgatev1alpha1.CloudflareTunnel, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: tunnel.Generation,
	}

	meta.SetStatusCondition(&tunnel.Status.Conditions, condition)
}
