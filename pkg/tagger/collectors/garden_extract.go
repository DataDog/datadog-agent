package collectors

import (
	"fmt"

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
				fmt.Sprintf("container_name:%s", gardenContainer.Handle()),
				fmt.Sprintf("app_instance_guid:%s", gardenContainer.Handle()),
			}
		}
	}
	return tagsByInstanceGUID, nil
}
