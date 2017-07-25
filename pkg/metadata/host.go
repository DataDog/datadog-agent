package metadata

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
)

// HostCollector fills and sends the old metadata payload used in the
// Agent v5
type HostCollector struct{}

// Send collects the data needed and submits the payload
func (hp *HostCollector) Send(fwd forwarder.Forwarder) error {
	var hostname string
	x, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostname"))
	if found {
		hostname = x.(string)
	}

	payload := v5.GetPayload(hostname)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to serialize host metadata payload, %s", err)
	}

	err = fwd.SubmitV1Intake(&payloadBytes)
	if err != nil {
		return fmt.Errorf("unable to submit host metadata payload to the forwarder, %s", err)
	}

	log.Infof("Sent host metadata payload, size: %d bytes.", len(payloadBytes))
	log.Debugf("Sent host metadata payload, content: %v", string(payloadBytes))

	return nil
}

func init() {
	catalog["host"] = new(HostCollector)
}
