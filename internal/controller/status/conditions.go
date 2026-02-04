// Package status provides condition management utilities for cfgate controllers.
// It adapts patterns from Envoy Gateway for consistent status handling across CRDs.
package status

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	// MaxConditionMessageLength is the maximum length of a condition message.
	// Messages longer than this will be truncated with an ellipsis.
	// Matches Gateway API and Kubernetes conventions.
	MaxConditionMessageLength = 32768
)

// Gateway API standard condition types.
const (
	// ConditionTypeReady indicates the resource is ready.
	// Used by Gateway, GatewayClass.
	ConditionTypeReady = "Ready"

	// ConditionTypeAccepted indicates the resource is accepted by the controller.
	// Used by Gateway, GatewayClass, Routes.
	ConditionTypeAccepted = "Accepted"

	// ConditionTypeProgrammed indicates the resource configuration is programmed.
	// Used by Gateway, Routes.
	ConditionTypeProgrammed = "Programmed"

	// ConditionTypeResolvedRefs indicates all references are resolved.
	// Used by Routes.
	ConditionTypeResolvedRefs = "ResolvedRefs"
)

// cfgate-specific condition types for CloudflareTunnel.
const (
	// ConditionTypeCredentialsValid indicates credentials are valid.
	ConditionTypeCredentialsValid = "CredentialsValid"

	// ConditionTypeTunnelCreated indicates tunnel exists in Cloudflare.
	ConditionTypeTunnelCreated = "TunnelCreated"

	// ConditionTypeTunnelConfigured indicates tunnel configuration is synced.
	ConditionTypeTunnelConfigured = "TunnelConfigured"

	// ConditionTypeDeploymentReady indicates cloudflared deployment is ready.
	ConditionTypeDeploymentReady = "DeploymentReady"
)

// cfgate-specific condition types for CloudflareDNS.
const (
	// ConditionTypeZonesResolved indicates zones are resolved via API.
	ConditionTypeZonesResolved = "ZonesResolved"

	// ConditionTypeRecordsSynced indicates DNS records are synced.
	ConditionTypeRecordsSynced = "RecordsSynced"

	// ConditionTypeOwnershipVerified indicates ownership TXT records verified.
	ConditionTypeOwnershipVerified = "OwnershipVerified"
)

// cfgate-specific condition types for CloudflareAccessPolicy.
const (
	// ConditionTypeTargetsResolved indicates target references are resolved.
	ConditionTypeTargetsResolved = "TargetsResolved"

	// ConditionTypeApplicationCreated indicates Access Application exists.
	ConditionTypeApplicationCreated = "ApplicationCreated"

	// ConditionTypePoliciesAttached indicates Access Policies are attached.
	ConditionTypePoliciesAttached = "PoliciesAttached"

	// ConditionTypeServiceTokensReady indicates service tokens are ready.
	ConditionTypeServiceTokensReady = "ServiceTokensReady"
)

// Policy condition types for Gateway API Policy status.
const (
	// PolicyConditionAccepted indicates policy is accepted by the controller.
	PolicyConditionAccepted = "Accepted"

	// PolicyReasonAccepted indicates policy was accepted.
	PolicyReasonAccepted = "Accepted"

	// PolicyReasonTargetNotFound indicates target resource not found.
	PolicyReasonTargetNotFound = "TargetNotFound"

	// PolicyReasonConflicted indicates policy conflicts with another.
	PolicyReasonConflicted = "Conflicted"

	// PolicyReasonInvalid indicates policy is invalid.
	PolicyReasonInvalid = "Invalid"
)

// Reasons for condition status changes.
const (
	// Common reasons.
	ReasonReconciling      = "Reconciling"
	ReasonReconcileSuccess = "ReconcileSuccess"
	ReasonReconcileError   = "ReconcileError"

	// Credentials reasons.
	ReasonCredentialsValid   = "CredentialsValid"
	ReasonCredentialsInvalid = "CredentialsInvalid"
	ReasonCredentialsMissing = "CredentialsMissing"

	// Tunnel reasons.
	ReasonTunnelCreated     = "TunnelCreated"
	ReasonTunnelAdopted     = "TunnelAdopted"
	ReasonTunnelCreateError = "TunnelCreateError"
	ReasonTunnelNotFound    = "TunnelNotFound"

	// Configuration reasons.
	ReasonConfigSynced    = "ConfigSynced"
	ReasonConfigSyncError = "ConfigSyncError"

	// Deployment reasons.
	ReasonDeploymentReady    = "DeploymentReady"
	ReasonDeploymentNotReady = "DeploymentNotReady"
	ReasonDeploymentError    = "DeploymentError"

	// DNS reasons.
	ReasonZonesResolved        = "ZonesResolved"
	ReasonZoneResolutionFailed = "ZoneResolutionFailed"
	ReasonRecordsSynced        = "RecordsSynced"
	ReasonRecordSyncFailed     = "RecordSyncFailed"
	ReasonOwnershipVerified    = "OwnershipVerified"
	ReasonOwnershipFailed      = "OwnershipFailed"

	// Access Policy reasons.
	ReasonTargetsResolved    = "TargetsResolved"
	ReasonTargetNotFound     = "TargetNotFound"
	ReasonApplicationCreated = "ApplicationCreated"
	ReasonApplicationError   = "ApplicationError"
	ReasonPoliciesAttached   = "PoliciesAttached"
	ReasonPolicyError        = "PolicyError"
	ReasonServiceTokensReady = "ServiceTokensReady"
	ReasonServiceTokenError  = "ServiceTokenError"
)

// MergeConditions merges condition updates into an existing condition slice.
// - Preserves LastTransitionTime when status unchanged
// - Truncates messages to MaxConditionMessageLength
// - Returns new slice (does not modify input)
func MergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
	if len(updates) == 0 {
		return conditions
	}

	now := metav1.NewTime(time.Now())
	result := make([]metav1.Condition, 0, len(conditions)+len(updates))

	// Index existing conditions by type
	existing := make(map[string]metav1.Condition, len(conditions))
	for _, c := range conditions {
		existing[c.Type] = c
	}

	// Track which types we've processed
	processed := make(map[string]bool, len(updates))

	// Process updates
	for _, update := range updates {
		update.Message = truncateConditionMessage(update.Message)

		if prev, found := existing[update.Type]; found {
			// Preserve LastTransitionTime if status unchanged
			if prev.Status == update.Status {
				update.LastTransitionTime = prev.LastTransitionTime
			} else {
				update.LastTransitionTime = now
			}
			// Preserve ObservedGeneration if not explicitly set
			if update.ObservedGeneration == 0 {
				update.ObservedGeneration = prev.ObservedGeneration
			}
		} else {
			update.LastTransitionTime = now
		}

		result = append(result, update)
		processed[update.Type] = true
	}

	// Keep unprocessed existing conditions
	for _, c := range conditions {
		if !processed[c.Type] {
			result = append(result, c)
		}
	}

	return result
}

// truncateConditionMessage truncates a message to MaxConditionMessageLength.
func truncateConditionMessage(msg string) string {
	if len(msg) <= MaxConditionMessageLength {
		return msg
	}
	// Leave room for ellipsis
	return msg[:MaxConditionMessageLength-3] + "..."
}

// Error2ConditionMsg converts an error to a human-readable condition message.
// - Capitalizes first letter
// - Ensures trailing period
// - Handles nil errors gracefully
func Error2ConditionMsg(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if len(msg) == 0 {
		return ""
	}

	// Capitalize first letter
	msg = strings.ToUpper(msg[:1]) + msg[1:]

	// Ensure trailing period
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}

	return msg
}

// NewCondition creates a new condition with proper timestamps.
func NewCondition(
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
	generation int64,
) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            truncateConditionMessage(message),
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: generation,
	}
}

// SetCondition sets or updates a condition in a slice.
// Returns the updated slice.
func SetCondition(conditions []metav1.Condition, condition metav1.Condition) []metav1.Condition {
	return MergeConditions(conditions, condition)
}

// FindCondition returns the condition with the given type, or nil if not found.
func FindCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// RemoveCondition removes the condition with the given type.
// Returns the updated slice.
func RemoveCondition(conditions []metav1.Condition, conditionType string) []metav1.Condition {
	result := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		if c.Type != conditionType {
			result = append(result, c)
		}
	}
	return result
}

// ConditionTrue returns true if the condition is True.
func ConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	c := FindCondition(conditions, conditionType)
	return c != nil && c.Status == metav1.ConditionTrue
}

// ConditionFalse returns true if the condition is False.
func ConditionFalse(conditions []metav1.Condition, conditionType string) bool {
	c := FindCondition(conditions, conditionType)
	return c != nil && c.Status == metav1.ConditionFalse
}

// ConditionUnknown returns true if the condition is Unknown or not found.
func ConditionUnknown(conditions []metav1.Condition, conditionType string) bool {
	c := FindCondition(conditions, conditionType)
	return c == nil || c.Status == metav1.ConditionUnknown
}

// --- CloudflareTunnel Condition Constructors ---

// NewCredentialsValidCondition creates a CredentialsValid condition.
func NewCredentialsValidCondition(valid bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if valid {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeCredentialsValid, status, reason, message, generation)
}

// NewTunnelCreatedCondition creates a TunnelCreated condition.
func NewTunnelCreatedCondition(created bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if created {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeTunnelCreated, status, reason, message, generation)
}

// NewTunnelConfiguredCondition creates a TunnelConfigured condition.
func NewTunnelConfiguredCondition(configured bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if configured {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeTunnelConfigured, status, reason, message, generation)
}

// NewDeploymentReadyCondition creates a DeploymentReady condition.
func NewDeploymentReadyCondition(ready bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeDeploymentReady, status, reason, message, generation)
}

// NewTunnelReadyCondition creates the overall Ready condition for CloudflareTunnel.
// Ready = CredentialsValid AND TunnelCreated AND TunnelConfigured AND DeploymentReady
func NewTunnelReadyCondition(conditions []metav1.Condition, generation int64) metav1.Condition {
	ready := ConditionTrue(conditions, ConditionTypeCredentialsValid) &&
		ConditionTrue(conditions, ConditionTypeTunnelCreated) &&
		ConditionTrue(conditions, ConditionTypeTunnelConfigured) &&
		ConditionTrue(conditions, ConditionTypeDeploymentReady)

	if ready {
		return NewCondition(ConditionTypeReady, metav1.ConditionTrue,
			ReasonReconcileSuccess, "Tunnel is ready.", generation)
	}

	// Find first failing condition for message
	for _, t := range []string{
		ConditionTypeCredentialsValid,
		ConditionTypeTunnelCreated,
		ConditionTypeTunnelConfigured,
		ConditionTypeDeploymentReady,
	} {
		c := FindCondition(conditions, t)
		if c != nil && c.Status != metav1.ConditionTrue {
			return NewCondition(ConditionTypeReady, metav1.ConditionFalse,
				c.Reason, c.Message, generation)
		}
	}

	return NewCondition(ConditionTypeReady, metav1.ConditionUnknown,
		ReasonReconciling, "Reconciling tunnel.", generation)
}

// --- CloudflareDNS Condition Constructors ---

// NewZonesResolvedCondition creates a ZonesResolved condition.
func NewZonesResolvedCondition(resolved bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if resolved {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeZonesResolved, status, reason, message, generation)
}

// NewRecordsSyncedCondition creates a RecordsSynced condition.
func NewRecordsSyncedCondition(synced bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if synced {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeRecordsSynced, status, reason, message, generation)
}

// NewOwnershipVerifiedCondition creates an OwnershipVerified condition.
func NewOwnershipVerifiedCondition(verified bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if verified {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeOwnershipVerified, status, reason, message, generation)
}

// NewDNSReadyCondition creates the overall Ready condition for CloudflareDNS.
// Ready = CredentialsValid AND ZonesResolved AND RecordsSynced
func NewDNSReadyCondition(conditions []metav1.Condition, generation int64) metav1.Condition {
	ready := ConditionTrue(conditions, ConditionTypeCredentialsValid) &&
		ConditionTrue(conditions, ConditionTypeZonesResolved) &&
		ConditionTrue(conditions, ConditionTypeRecordsSynced)

	if ready {
		return NewCondition(ConditionTypeReady, metav1.ConditionTrue,
			ReasonReconcileSuccess, "DNS sync is ready.", generation)
	}

	// Find first failing condition for message
	for _, t := range []string{
		ConditionTypeCredentialsValid,
		ConditionTypeZonesResolved,
		ConditionTypeRecordsSynced,
	} {
		c := FindCondition(conditions, t)
		if c != nil && c.Status != metav1.ConditionTrue {
			return NewCondition(ConditionTypeReady, metav1.ConditionFalse,
				c.Reason, c.Message, generation)
		}
	}

	return NewCondition(ConditionTypeReady, metav1.ConditionUnknown,
		ReasonReconciling, "Reconciling DNS sync.", generation)
}

// --- CloudflareAccessPolicy Condition Constructors ---

// NewTargetsResolvedCondition creates a TargetsResolved condition.
func NewTargetsResolvedCondition(resolved bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if resolved {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeTargetsResolved, status, reason, message, generation)
}

// NewApplicationCreatedCondition creates an ApplicationCreated condition.
func NewApplicationCreatedCondition(created bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if created {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeApplicationCreated, status, reason, message, generation)
}

// NewPoliciesAttachedCondition creates a PoliciesAttached condition.
func NewPoliciesAttachedCondition(attached bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if attached {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypePoliciesAttached, status, reason, message, generation)
}

// NewServiceTokensReadyCondition creates a ServiceTokensReady condition.
func NewServiceTokensReadyCondition(ready bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	return NewCondition(ConditionTypeServiceTokensReady, status, reason, message, generation)
}

// NewAccessPolicyReadyCondition creates the overall Ready condition for CloudflareAccessPolicy.
// Ready = CredentialsValid AND TargetsResolved AND ApplicationCreated AND PoliciesAttached
// ServiceTokensReady is optional (only required if serviceTokens configured)
func NewAccessPolicyReadyCondition(conditions []metav1.Condition, hasServiceTokens bool, generation int64) metav1.Condition {
	ready := ConditionTrue(conditions, ConditionTypeCredentialsValid) &&
		ConditionTrue(conditions, ConditionTypeTargetsResolved) &&
		ConditionTrue(conditions, ConditionTypeApplicationCreated) &&
		ConditionTrue(conditions, ConditionTypePoliciesAttached)

	// ServiceTokensReady required only if service tokens configured
	if hasServiceTokens {
		ready = ready && ConditionTrue(conditions, ConditionTypeServiceTokensReady)
	}

	if ready {
		return NewCondition(ConditionTypeReady, metav1.ConditionTrue,
			ReasonReconcileSuccess, "Access policy is ready.", generation)
	}

	// Find first failing condition for message
	checkOrder := []string{
		ConditionTypeCredentialsValid,
		ConditionTypeTargetsResolved,
		ConditionTypeApplicationCreated,
		ConditionTypePoliciesAttached,
	}
	if hasServiceTokens {
		checkOrder = append(checkOrder, ConditionTypeServiceTokensReady)
	}

	for _, t := range checkOrder {
		c := FindCondition(conditions, t)
		if c != nil && c.Status != metav1.ConditionTrue {
			return NewCondition(ConditionTypeReady, metav1.ConditionFalse,
				c.Reason, c.Message, generation)
		}
	}

	return NewCondition(ConditionTypeReady, metav1.ConditionUnknown,
		ReasonReconciling, "Reconciling access policy.", generation)
}

// NewPolicyAcceptedCondition creates an Accepted condition for policy status.
func NewPolicyAcceptedCondition(accepted bool, reason, message string, generation int64) metav1.Condition {
	status := metav1.ConditionFalse
	if accepted {
		status = metav1.ConditionTrue
	}
	return NewCondition(PolicyConditionAccepted, status, reason, message, generation)
}

// --- Gateway API PolicyStatus Helpers ---

// NewPolicyAncestorStatus creates a PolicyAncestorStatus for a target reference.
func NewPolicyAncestorStatus(
	ancestorRef gwapiv1.ParentReference,
	controllerName gwapiv1.GatewayController,
	conditions []metav1.Condition,
) gwapiv1.PolicyAncestorStatus {
	return gwapiv1.PolicyAncestorStatus{
		AncestorRef:    ancestorRef,
		ControllerName: controllerName,
		Conditions:     conditions,
	}
}

// SetPolicyAncestorStatus sets or updates an ancestor status in PolicyStatus.
func SetPolicyAncestorStatus(
	status *gwapiv1.PolicyStatus,
	ancestorStatus gwapiv1.PolicyAncestorStatus,
) {
	if status == nil {
		return
	}

	// Find existing ancestor status
	for i, existing := range status.Ancestors {
		if ancestorRefEqual(existing.AncestorRef, ancestorStatus.AncestorRef) &&
			existing.ControllerName == ancestorStatus.ControllerName {
			status.Ancestors[i] = ancestorStatus
			return
		}
	}

	// Not found, append
	status.Ancestors = append(status.Ancestors, ancestorStatus)
}

// ancestorRefEqual compares two ParentReferences for equality.
func ancestorRefEqual(a, b gwapiv1.ParentReference) bool {
	// Compare Group (default: gateway.networking.k8s.io)
	aGroup := gwapiv1.GroupName
	if a.Group != nil {
		aGroup = string(*a.Group)
	}
	bGroup := gwapiv1.GroupName
	if b.Group != nil {
		bGroup = string(*b.Group)
	}
	if aGroup != bGroup {
		return false
	}

	// Compare Kind (default: Gateway)
	aKind := "Gateway"
	if a.Kind != nil {
		aKind = string(*a.Kind)
	}
	bKind := "Gateway"
	if b.Kind != nil {
		bKind = string(*b.Kind)
	}
	if aKind != bKind {
		return false
	}

	// Compare Name
	return a.Name == b.Name
}

// --- Logging Patterns ---

// LogConditionChange logs a condition change at Info level.
func LogConditionChange(log logr.Logger, resource, conditionType string, old, new metav1.ConditionStatus, reason string) {
	if old != new {
		log.Info("condition status changed",
			"resource", resource,
			"condition", conditionType,
			"old", old,
			"new", new,
			"reason", reason,
		)
	}
}

// LogStatusUpdate logs a status update at V(1) debug level.
func LogStatusUpdate(log logr.Logger, resource string, conditions []metav1.Condition) {
	log.V(1).Info("updating status",
		"resource", resource,
		"conditionCount", len(conditions),
	)
}
