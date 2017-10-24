package ec2

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("EC2 Metadata availability", new(ec2MetadataAvailabilityDiagnosis))
}

type ec2MetadataAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *ec2MetadataAvailabilityDiagnosis) Diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
