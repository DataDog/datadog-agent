package collectors

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *GardenCollector) extractTags(nodename string) (tagsByInstanceGUID map[string][]string, err error) {
	if c.clusterAgentEnabled {
		tagsByInstanceGUID, err = c.dcaClient.GetCFAppsMetadataForNode(nodename)
		if err != nil {
			return tagsByInstanceGUID, err
		}
	} else {
		log.Debug("Cluster agent not enabled or misconfigured, tagging CF app with container id only")
		gardenContainers, err := c.gardenUtil.GetGardenContainers()
		if err != nil {
			return tagsByInstanceGUID, fmt.Errorf("cannot get container list from local garden API: %v", err)
		}
		tagsByInstanceGUID = make(map[string][]string, len(gardenContainers))
		for _, gardenContainer := range gardenContainers {
			tagsByInstanceGUID[gardenContainer.Handle()] = []string{
				fmt.Sprintf("%s:%s", cloudfoundry.ContainerNameTagKey, gardenContainer.Handle()),
				fmt.Sprintf("%s:%s", cloudfoundry.AppInstanceGUIDTagKey, gardenContainer.Handle()),
			}
		}
	}
	return tagsByInstanceGUID, nil
}
