package metadata

import (
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// HostCollector fills and sends the old metadata payload used in the
// Agent v5
type HostCollector struct{}

// Send collects the data needed and submits the payload
func (hp *HostCollector) Send(s *serializer.Serializer) error {
	var hostname string
	x, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostname"))
	if found {
		hostname = x.(string)
	}

	payload := v5.GetPayload(hostname)
	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit host metadata payload, %s", err)
	}
	return nil
}

func init() {
	catalog["host"] = new(HostCollector)
}
