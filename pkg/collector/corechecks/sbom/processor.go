// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sbom

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/DataDog/agent-payload/v5/sbom"
	model "github.com/DataDog/agent-payload/v5/sbom"
)

var /* const */ (
	sourceAgent = "agent"
)

type processor struct {
	queue chan *model.SBOMEntity
}

func newProcessor(sender aggregator.Sender, maxNbItem int, maxRetentionTime time.Duration) *processor {
	return &processor{
		queue: queue.NewQueue(maxNbItem, maxRetentionTime, func(entities []*model.SBOMEntity) {
			sender.SBOM([]sbom.SBOMPayload{
				{
					Version:  1,
					Source:   &sourceAgent,
					Entities: entities,
				},
			})
		}),
	}
}

func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		p.processSBOM(event.Entity.(*workloadmeta.ContainerImageMetadata))
	}
}

func (p *processor) processRefresh(allImages []*workloadmeta.ContainerImageMetadata) {
	// So far, the check is refreshing all the images every 5 minutes all together.
	for _, img := range allImages {
		p.processSBOM(img)
	}
}

func (p *processor) processSBOM(img *workloadmeta.ContainerImageMetadata) {
	if img.CycloneDXBOM == nil {
		return
	}

	p.queue <- &model.SBOMEntity{
		Type:        model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
		Id:          img.ID,
		GeneratedAt: nil,
		Tags:        img.RepoTags,
		InUse:       true, // TODO: compute this field
		Sbom: &sbom.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(img.CycloneDXBOM),
		},
	}
}

func (p *processor) stop() {
	close(p.queue)
}
