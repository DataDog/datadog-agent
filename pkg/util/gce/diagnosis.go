package gce

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("GCE Metadata availability", new(gceMetadataAvailabilityDiagnosis))
}

type gceMetadataAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *gceMetadataAvailabilityDiagnosis) Diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
