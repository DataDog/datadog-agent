package metadata

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
)

// ResourcesCollector sends the old metadata payload used in the
// Agent v5
type ResourcesCollector struct{}

// Send collects the data needed and submits the payload
func (rp *ResourcesCollector) Send(apiKey string, fwd forwarder.Forwarder) error {
	var hostname string
	x, found := util.Cache.Get("hostname")
	if found {
		hostname = x.(string)
	}

	payload := map[string]interface{}{
		"resources": resources.GetPayload(hostname),
	}
	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return fmt.Errorf("unable to serialize processes metadata payload, %s", err)
	}

	err = fwd.SubmitV1Intake(apiKey, &payloadBytes)
	if err != nil {
		return fmt.Errorf("unable to submit processes metadata payload to the forwarder, %s", err)
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payloadBytes))
	log.Debugf("Sent processes metadata payload, content: %v", string(payloadBytes))

	return nil
}

func init() {
	catalog["resources"] = new(ResourcesCollector)
}
