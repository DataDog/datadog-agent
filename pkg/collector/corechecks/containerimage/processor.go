// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	model "github.com/DataDog/agent-payload/v5/contimage"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type processor struct {
	queue chan *model.ContainerImage
}

func newProcessor(sender aggregator.Sender, maxNbItem int, maxRetentionTime time.Duration) *processor {
	return &processor{
		queue: queue.NewQueue(maxNbItem, maxRetentionTime, func(images []*model.ContainerImage) {
			sender.ContainerImage([]model.ContainerImagePayload{
				{
					Version: "v1",
					Images:  images,
				},
			})
		}),
	}
}

func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		p.processImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
	}
}

func (p *processor) processRefresh(allImages []*workloadmeta.ContainerImageMetadata) {
	// So far, the check is refreshing all the images every 5 minutes all together.
	for _, img := range allImages {
		p.processImage(img)
	}
}

func (p *processor) processImage(img *workloadmeta.ContainerImageMetadata) {
	var lastCreated *timestamppb.Timestamp = nil
	layers := make([]*model.ContainerImage_ContainerImageLayer, 0, len(img.Layers))
	for _, layer := range img.Layers {
		var created *timestamppb.Timestamp = nil
		if layer.History.Created != nil {
			created = timestamppb.New(*layer.History.Created)
			lastCreated = created
		}

		layers = append(layers, &model.ContainerImage_ContainerImageLayer{
			Urls:      layer.URLs,
			MediaType: layer.MediaType,
			Digest:    layer.Digest,
			Size:      layer.SizeBytes,
			History: &model.ContainerImage_ContainerImageLayer_History{
				Created:    created,
				CreatedBy:  layer.History.CreatedBy,
				Author:     layer.History.Author,
				Comment:    layer.History.Comment,
				EmptyLayer: layer.History.EmptyLayer,
			},
		})
	}

	for _, repoDigest := range img.RepoDigests {
		repo := strings.SplitN(repoDigest, "@sha256:", 2)[0]
		repoSplitted := strings.Split(repo, "/")
		registry := ""
		if len(repoSplitted) > 2 {
			registry = repoSplitted[0]
		}
		shortName := repoSplitted[len(repoSplitted)-1]

		id := repo + "@" + img.ID

		tags := make([]string, 0, len(img.RepoTags))
		for _, repoTag := range img.RepoTags {
			if strings.HasPrefix(repoTag, repo+":") {
				tags = append(tags, strings.SplitN(repoTag, ":", 2)[1])
			}
		}

		repoDigests := make([]string, 0, 1)
		for _, repoDigest := range img.RepoDigests {
			if strings.HasPrefix(repoDigest, repo+"@sha256:") {
				repoDigests = append(repoDigests, repoDigest)
			}
		}

		p.queue <- &model.ContainerImage{
			Id:          id,
			Name:        repo,
			Registry:    registry,
			ShortName:   shortName,
			Tags:        tags,
			Digest:      img.ID,
			Size:        img.SizeBytes,
			RepoDigests: repoDigests,
			Os: &model.ContainerImage_OperatingSystem{
				Name:         img.OS,
				Version:      img.OSVersion,
				Architecture: img.Architecture,
			},
			Layers:  layers,
			BuiltAt: lastCreated,
		}
	}
}

func (p *processor) stop() {
	close(p.queue)
}
