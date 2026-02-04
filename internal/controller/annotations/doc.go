// Package annotations provides annotation parsing utilities for cfgate controllers.
//
// It centralizes annotation constants and parsing logic for consistent handling
// across all cfgate route controllers:
//   - HTTPRoute
//   - TCPRoute
//   - UDPRoute
//   - GRPCRoute
//
// Annotation placement follows Gateway API semantics per external-dns patterns:
//   - Infrastructure-level annotations (tunnel target) go on Gateway/CloudflareTunnel
//   - Application-level annotations (protocol, ttl, proxied) go on Routes
//
// The package provides:
//   - Constants for all cfgate annotation keys
//   - Parsing functions with sensible defaults (GetAnnotation, GetAnnotationBool, etc.)
//   - Validation functions for user feedback (ValidateOriginProtocol, ValidateTTL, etc.)
//   - ValidationResult aggregation for route validation
//
// Example usage:
//
//	protocol := annotations.GetAnnotation(route, annotations.AnnotationOriginProtocol)
//	if err := annotations.ValidateOriginProtocol(protocol); err != nil {
//	    log.Info("invalid protocol", "error", err.Error())
//	}
//
//	sslVerify := annotations.GetAnnotationBool(route, annotations.AnnotationOriginSSLVerify, true)
//	timeout := annotations.GetAnnotationDuration(route, annotations.AnnotationOriginConnectTimeout, 30*time.Second)
//
// TCPRoute and UDPRoute require the hostname annotation since Gateway API has no
// spec.hostnames field for L4 routes:
//
//	result := annotations.ValidateRouteAnnotations(tcpRoute, true) // requireHostname=true
//	if !result.Valid {
//	    // Handle validation errors
//	}
package annotations
