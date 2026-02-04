// Package status provides condition management utilities for cfgate controllers.
//
// It centralizes condition management logic to ensure consistent status handling
// across all cfgate CRDs:
//   - CloudflareTunnel
//   - CloudflareDNS
//   - CloudflareAccessPolicy
//
// The package adapts patterns from Envoy Gateway for condition merging,
// message formatting, and Gateway API PolicyStatus handling.
//
// Core Functions:
//   - MergeConditions: Merge condition updates preserving LastTransitionTime
//   - Error2ConditionMsg: Format errors for human-readable condition messages
//   - NewCondition: Generic condition constructor
//   - FindCondition: Lookup condition by type
//   - SetCondition/RemoveCondition: Slice manipulation helpers
//
// Domain-Specific Builders:
//   - CloudflareTunnel: NewCredentialsValidCondition, NewTunnelCreatedCondition, etc.
//   - CloudflareAccessPolicy: NewTargetsResolvedCondition, NewApplicationCreatedCondition, etc.
//   - Gateway API Policy: NewPolicyAncestorStatus, SetPolicyAncestorStatus
//
// Example usage:
//
//	conditions = status.MergeConditions(conditions,
//	    status.NewCredentialsValidCondition(true,
//	        status.ReasonCredentialsValid,
//	        "API token validated successfully.",
//	        generation,
//	    ),
//	)
//	readyCondition := status.NewTunnelReadyCondition(conditions, generation)
//	conditions = status.MergeConditions(conditions, readyCondition)
package status
