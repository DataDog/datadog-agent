package metadata

import (
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// ResourcesCollector sends the old metadata payload used in the
// Agent v5
type ResourcesCollector struct{}

// Send collects the data needed and submits the payload
func (rp *ResourcesCollector) Send(s *serializer.Serializer) error {
	var hostname string
	x, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostname"))
	if found {
		hostname = x.(string)
	}

	payload := map[string]interface{}{
		"resources": resources.GetPayload(hostname),
	}
	if err := s.SendJSONToV1Intake(payload); err != nil {
		return fmt.Errorf("unable to serialize processes metadata payload, %s", err)
	}
	return nil
}

func init() {
	catalog["resources"] = new(ResourcesCollector)
}
