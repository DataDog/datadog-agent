package ec2

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("EC2 Metadata availability", diagnose)
}

// diagnose the docker availability on the system
func diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
