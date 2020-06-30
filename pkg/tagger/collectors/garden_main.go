// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-Present Datadog, Inc.

package collectors

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	cfCollectorName = "cloudfoundry"
)

// GardenCollector collects tags for CF application containers
type GardenCollector struct {
	infoOut             chan<- []*TagInfo
	dcaClient           clusteragent.DCAClientInterface
	gardenUtil          cloudfoundry.GardenUtilInterface
	clusterAgentEnabled bool
}

// Detect tries to connect to the Garden API and the cluster agent
func (c *GardenCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {

	// Detect if we're on a compute VM by trying to connect to the local garden API
	var err error
	c.gardenUtil, err = cloudfoundry.GetGardenUtil()
	if err != nil {
		if retry.IsErrWillRetry(err) {
			return NoCollection, err
		}
		return NoCollection, err
	}

	// if DCA is enabled and can't communicate with the DCA, let the tagger retry.
	var errDCA error
	if config.Datadog.GetBool("cluster_agent.enabled") {
		c.clusterAgentEnabled = false
		c.dcaClient, errDCA = clusteragent.GetClusterAgentClient()
		if errDCA != nil {
			log.Errorf("Could not initialise the communication with the cluster agent: %s", errDCA.Error())
			// continue to retry while we can
			if retry.IsErrPermaFail(errDCA) {
				log.Error("Permanent failure in communication with the cluster agent")
			}
			return NoCollection, errDCA
		}
		c.clusterAgentEnabled = true
	}

	c.infoOut = out
	return PullCollection, nil
}

// Pull gets the list of containers
func (c *GardenCollector) Pull() error {
	var tagsByInstanceGUID map[string][]string
	var tagInfo []*TagInfo
	tagsByInstanceGUID, err := c.extractTags(config.Datadog.GetString("bosh_id"))
	if err != nil {
		return err
	}
	for handle, tags := range tagsByInstanceGUID {
		entity := containers.BuildTaggerEntityName(handle)
		tagInfo = append(tagInfo, &TagInfo{
			Source:       cfCollectorName,
			Entity:       entity,
			HighCardTags: tags,
		})
	}
	c.infoOut <- tagInfo
	return nil
}

// Fetch gets the tags for a specific entity
func (c *GardenCollector) Fetch(entity string) ([]string, []string, []string, error) {
	_, cid := containers.SplitEntityName(entity)
	tagsByInstanceGUID, err := c.extractTags(config.Datadog.GetString("bosh_id"))
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}
	tags, ok := tagsByInstanceGUID[cid]
	if !ok {
		return []string{}, []string{}, []string{}, fmt.Errorf("could not find tags for app %s", cid)
	}
	return []string{}, []string{}, tags, nil
}

func gardenFactory() Collector {
	return &GardenCollector{}
}

func init() {
	registerCollector(cfCollectorName, gardenFactory, NodeRuntime)
}
