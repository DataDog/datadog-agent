package cloudfoundry

import (
	"code.cloudfoundry.org/garden"
	"context"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strings"
)
import "github.com/DataDog/datadog-agent/pkg/workloadmeta"

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

	log.Infof("Cloud Foundry container tagger started successfully!")
	for {
		select {
		case bundle := <-ch:
			// close Ch to indicate that the Store can proceed to the next subscriber
			close(bundle.Ch)

			for _, evt := range bundle.Events {
				if evt.Type == workloadmeta.EventTypeSet {
					containerID := evt.Entity.GetID().ID
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

					log.Infof("Injecting tags into container %s", containerID)

					container, err := gardenUtil.GetContainer(containerID)
					if err != nil {
						log.Warnf("Error retrieving container %s from the garden api: %v", containerID, err)
						continue
					}

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
						log.Debugf("Process %s exit code: %d", p.ID(), exitCode)
					}()
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
