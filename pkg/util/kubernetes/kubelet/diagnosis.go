package kubelet

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("Kubelet availability", new(kubeletDiagnosis))
}

type kubeletDiagnosis struct{}

// Diagnosee the API server availability
func (dd *kubeletDiagnosis) Diagnose() error {
	_, err := locateKubelet()
	if err != nil {
		log.Error(err)
	}
	return err
}
