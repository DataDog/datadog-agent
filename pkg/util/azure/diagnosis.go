package azure

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("Azure Metadata availability", new(azureMetadataAvailabilityDiagnosis))
}

type azureMetadataAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *azureMetadataAvailabilityDiagnosis) Diagnose() error {
	_, err := GetHostAlias()
	if err != nil {
		log.Error(err)
	}
	return err
}
