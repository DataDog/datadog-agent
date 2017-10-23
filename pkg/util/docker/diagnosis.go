// +build docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("Docker availability", new(dockerAvailabilityDiagnosis))
}

type dockerAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dockerAvailabilityDiagnosis) Diagnose() error {
	_, err := ConnectToDocker()
	if err != nil {
		log.Error(err)
	}
	return err
}
