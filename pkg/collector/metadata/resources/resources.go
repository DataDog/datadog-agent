package resources

import (
	"github.com/DataDog/gohai/processes"
	log "github.com/cihub/seelog"
)

// GetPayload builds a payload of processes metadata collected from gohai.
func GetPayload(hostname string) *Payload {

	// Get processes metadata from gohai
	proc, err := new(processes.Processes).Collect()
	if err != nil {
		log.Errorf("Failed to retrieve processes metadata: %s", err)
		return &Payload{}
	}

	processesPayload := map[string]interface{}{
		"snaps": []interface{}{proc},
	}

	return &Payload{
		Processes: processesPayload,
		Meta: map[string]string{
			"host": hostname,
		},
	}
}
