// Package features provides CRD detection and feature flags for optional
// Gateway API resources, enabling graceful degradation when experimental
// CRDs are unavailable.
package features

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// Gateway API group constant.
const (
	// GatewayAPIGroup is the API group for Gateway API resources.
	GatewayAPIGroup = "gateway.networking.k8s.io"
)

// Gateway API version constants.
const (
	// V1Alpha2 is the experimental channel version.
	V1Alpha2 = "v1alpha2"
	// V1Beta1 is the standard channel version.
	V1Beta1 = "v1beta1"
	// V1 is the GA version.
	V1 = "v1"
)

// Gateway API resource names.
const (
	TCPRouteResource        = "tcproutes"
	UDPRouteResource        = "udproutes"
	GRPCRouteResource       = "grpcroutes"
	ReferenceGrantResource  = "referencegrants"
)

// FeatureGates tracks which optional Gateway API CRDs are available.
// CRD availability is detected once at startup and cached for the
// controller lifetime (CRDs don't change at runtime in practice).
type FeatureGates struct {
	// TCPRouteCRDExists indicates TCPRoute (v1alpha2) is installed.
	// Required for TCP proxy support via Cloudflare Spectrum.
	TCPRouteCRDExists bool

	// UDPRouteCRDExists indicates UDPRoute (v1alpha2) is installed.
	// Required for UDP proxy support via Cloudflare Spectrum.
	UDPRouteCRDExists bool

	// GRPCRouteCRDExists indicates GRPCRoute (v1) is installed.
	// While GA, may not be installed in minimal Gateway API deployments.
	GRPCRouteCRDExists bool

	// ReferenceGrantCRDExists indicates ReferenceGrant (v1beta1) is installed.
	// Required for cross-namespace secret/service references.
	ReferenceGrantCRDExists bool
}

// DetectFeatures checks for the existence of optional Gateway API CRDs
// using the discovery client. Results are cached in FeatureGates.
// Each CRD check is independent; detection failures disable that feature.
func DetectFeatures(dc discovery.DiscoveryInterface) (*FeatureGates, error) {
	gates := &FeatureGates{}

	// Check TCPRoute (experimental channel)
	gates.TCPRouteCRDExists = crdExists(dc, schema.GroupVersionResource{
		Group:    GatewayAPIGroup,
		Version:  V1Alpha2,
		Resource: TCPRouteResource,
	})

	// Check UDPRoute (experimental channel)
	gates.UDPRouteCRDExists = crdExists(dc, schema.GroupVersionResource{
		Group:    GatewayAPIGroup,
		Version:  V1Alpha2,
		Resource: UDPRouteResource,
	})

	// Check GRPCRoute (GA but optional)
	gates.GRPCRouteCRDExists = crdExists(dc, schema.GroupVersionResource{
		Group:    GatewayAPIGroup,
		Version:  V1,
		Resource: GRPCRouteResource,
	})

	// Check ReferenceGrant (standard channel)
	gates.ReferenceGrantCRDExists = crdExists(dc, schema.GroupVersionResource{
		Group:    GatewayAPIGroup,
		Version:  V1Beta1,
		Resource: ReferenceGrantResource,
	})

	return gates, nil
}

// crdExists checks if a CRD is installed by attempting to list its resources.
// Returns true if the resource exists, false otherwise.
func crdExists(dc discovery.DiscoveryInterface, gvr schema.GroupVersionResource) bool {
	resources, err := dc.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		// Group/version not found means CRD not installed
		return false
	}

	// Verify the specific resource exists within the group
	for _, r := range resources.APIResources {
		if r.Name == gvr.Resource {
			return true
		}
	}
	return false
}

// HasTCPRouteSupport returns true if TCPRoute CRD is available.
func (g *FeatureGates) HasTCPRouteSupport() bool {
	return g.TCPRouteCRDExists
}

// HasUDPRouteSupport returns true if UDPRoute CRD is available.
func (g *FeatureGates) HasUDPRouteSupport() bool {
	return g.UDPRouteCRDExists
}

// HasGRPCRouteSupport returns true if GRPCRoute CRD is available.
func (g *FeatureGates) HasGRPCRouteSupport() bool {
	return g.GRPCRouteCRDExists
}

// HasReferenceGrantSupport returns true if ReferenceGrant CRD is available.
func (g *FeatureGates) HasReferenceGrantSupport() bool {
	return g.ReferenceGrantCRDExists
}

// SupportedRouteKinds returns the list of supported route kinds.
// Always includes HTTPRoute; conditionally includes TCP/UDP/GRPC.
func (g *FeatureGates) SupportedRouteKinds() []string {
	kinds := []string{"HTTPRoute"}
	if g.TCPRouteCRDExists {
		kinds = append(kinds, "TCPRoute")
	}
	if g.UDPRouteCRDExists {
		kinds = append(kinds, "UDPRoute")
	}
	if g.GRPCRouteCRDExists {
		kinds = append(kinds, "GRPCRoute")
	}
	return kinds
}

// LogFeatures logs the detected feature availability at startup.
// Called once during manager initialization.
func (g *FeatureGates) LogFeatures(log logr.Logger) {
	log.Info("Gateway API feature detection complete",
		"tcpRouteAvailable", g.TCPRouteCRDExists,
		"udpRouteAvailable", g.UDPRouteCRDExists,
		"grpcRouteAvailable", g.GRPCRouteCRDExists,
		"referenceGrantAvailable", g.ReferenceGrantCRDExists,
	)

	// Log warnings for missing experimental features at V(1) level
	if !g.TCPRouteCRDExists {
		log.V(1).Info("TCPRoute CRD not found, TCP routing disabled",
			"requiredVersion", V1Alpha2,
			"installHint", "Install Gateway API experimental channel CRDs",
		)
	}

	if !g.UDPRouteCRDExists {
		log.V(1).Info("UDPRoute CRD not found, UDP routing disabled",
			"requiredVersion", V1Alpha2,
			"installHint", "Install Gateway API experimental channel CRDs",
		)
	}

	if !g.ReferenceGrantCRDExists {
		log.V(1).Info("ReferenceGrant CRD not found, cross-namespace references disabled",
			"requiredVersion", V1Beta1,
			"installHint", "Install Gateway API standard channel CRDs",
		)
	}
}
