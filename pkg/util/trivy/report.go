package trivy

import (
	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
)

// TrivyReport describes a trivy report along with its marshaler
type TrivyReport struct {
	types.Report
	marshaler *cyclonedx.Marshaler
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *TrivyReport) ToCycloneDX() (*cyclonedxgo.BOM, error) {
	bom, err := r.marshaler.Marshal(r.Report)
	if err != nil {
		return nil, err
	}

	// We don't need the dependencies attribute. Remove to save memory.
	bom.Dependencies = nil
	return bom, nil
}
