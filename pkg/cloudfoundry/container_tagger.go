package cloudfoundry

import (
	"context"
	"strings"

	"code.cloudfoundry.org/garden"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	SharedNodeAgentTagsFile = "/home/vcap/app/.datadog/node_agent_tags.txt"
	componentName           = "cloudfoundry-container-tagger"
)

func StartContainerTagger(ctx context.Context) error {
	var err error
	gardenUtil, err := cloudfoundry.GetGardenUtil()
	if err != nil {
		return err
	}

	filter := workloadmeta.NewFilter([]workloadmeta.Kind{workloadmeta.KindContainer}, workloadmeta.SourceClusterOrchestrator, workloadmeta.EventTypeAll)
	store := workloadmeta.GetGlobalStore()
	ch := store.Subscribe(componentName, workloadmeta.NormalPriority, filter)
	defer store.Unsubscribe(ch)

	seen := make(map[string]bool)
	tagsHashByContainerID := make(map[string]string)

	log.Infof("CloudFoundry container tagger successfully started")
	for {
		select {
		case bundle := <-ch:
			// close Ch to indicate that the Store can proceed to the next subscriber
			close(bundle.Ch)

			for _, evt := range bundle.Events {
				containerID := evt.Entity.GetID().ID
				if evt.Type == workloadmeta.EventTypeSet {
					storeContainer, err := store.GetContainer(containerID)
					if err != nil {
						log.Warnf("Error retrieving container %s from the workloadmeta store: %v", containerID, err)
						continue
					}

					// extract tags
					tags := storeContainer.CollectorTags
					hostTags := host.GetHostTags(context.TODO(), true)
					tags = append(tags, hostTags.System...)
					tags = append(tags, hostTags.GoogleCloudPlatform...)

					hash := utils.ComputeTagsHash(tags)

					tagsHashByContainerID[containerID] = hash

					// check if the tags were already injected
					exist := seen[hash]

					// skip to the next event
					if exist {
						continue
					}

					// mark as seen
					seen[hash] = true

					container, err := gardenUtil.GetContainer(containerID)
					if err != nil {
						log.Warnf("Error retrieving container %s from the garden API: %v", containerID, err)
						continue
					}

					log.Debugf("Writing tags into container %s", containerID)

					// write tags into a file inside the container
					p, err := container.Run(garden.ProcessSpec{
						Path: "/usr/bin/tee",
						Args: []string{SharedNodeAgentTagsFile},
						User: "vcap",
					}, garden.ProcessIO{
						Stdin: strings.NewReader(strings.Join(tags, ",")),
					})
					if err != nil {
						log.Warnf("Error running a process inside container %s: %v", containerID, err)
						continue
					}
					go func() {
						exitCode, err := p.Wait()
						if err != nil {
							log.Warnf("Error while running process %s inside container %s: %v", p.ID(), containerID, err)
						}
						log.Debugf("Process %s under container %s exited with code: %d", containerID, p.ID(), exitCode)
					}()
				} else if evt.Type == workloadmeta.EventTypeUnset {
					hash := tagsHashByContainerID[containerID]
					delete(seen, hash)
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
