// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	model "github.com/DataDog/agent-payload/v5/contimage"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var /* const */ (
	sourceAgent = "agent"
)

type processor struct {
	queue chan *model.ContainerImage
}

func newProcessor(sender sender.Sender, maxNbItem int, maxRetentionTime time.Duration) *processor {
	return &processor{
		queue: queue.NewQueue(maxNbItem, maxRetentionTime, func(images []*model.ContainerImage) {
			encoded, err := proto.Marshal(&model.ContainerImagePayload{
				Version: "v1",
				Source:  &sourceAgent,
				Images:  images,
			})
			if err != nil {
				log.Errorf("Unable to encode message: %+v", err)
				return
			}

			sender.EventPlatformEvent(encoded, epforwarder.EventTypeContainerImages)
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
	ddTags, err := tagger.Tag("container_image_metadata://"+img.ID, collectors.HighCardinality)
	if err != nil {
		log.Errorf("Could not retrieve tags for container image %s: %v", img.ID, err)
	}

	var lastCreated *timestamppb.Timestamp
	layers := make([]*model.ContainerImage_ContainerImageLayer, 0, len(img.Layers))
	for _, layer := range img.Layers {
		var created *timestamppb.Timestamp
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

	// In containerd some images are created without a repo digest, and it's
	// also possible to remove repo digests manually.
	// This means that the set of repos that we need to handle is the union of
	// the repos present in the repo digests and the ones present in the repo
	// tags.
	repos := make(map[string]struct{})
	for _, repoDigest := range img.RepoDigests {
		repos[strings.SplitN(repoDigest, "@sha256:", 2)[0]] = struct{}{}
	}
	for _, repoTag := range img.RepoTags {
		repos[strings.SplitN(repoTag, ":", 2)[0]] = struct{}{}
	}

	for repo := range repos {
		repoSplitted := strings.Split(repo, "/")
		registry := ""
		if len(repoSplitted) > 2 {
			registry = repoSplitted[0]
		}
		shortName := repoSplitted[len(repoSplitted)-1]

		id := repo + "@" + img.ID

		repoTags := make([]string, 0, len(img.RepoTags))
		for _, repoTag := range img.RepoTags {
			if strings.HasPrefix(repoTag, repo+":") {
				repoTags = append(repoTags, strings.SplitN(repoTag, ":", 2)[1])
			}
		}

		repoDigests := make([]string, 0, 1)
		for _, repoDigest := range img.RepoDigests {
			if strings.HasPrefix(repoDigest, repo+"@sha256:") {
				repoDigests = append(repoDigests, repoDigest)
			}
		}

		// Because we split a single image entity into different payloads if it has several repo digests,
		// me must re-compute `image_id`, `image_name`, `short_image` and `image_tag` tags.
		ddTags2 := make([]string, 0, len(ddTags))
		for _, ddTag := range ddTags {
			if !strings.HasPrefix(ddTag, "image_id:") &&
				!strings.HasPrefix(ddTag, "image_name:") &&
				!strings.HasPrefix(ddTag, "short_image:") &&
				!strings.HasPrefix(ddTag, "image_tag:") {
				ddTags2 = append(ddTags2, ddTag)
			}
		}

		ddTags2 = append(ddTags2,
			"image_id:"+id,
			"image_name:"+repo,
			"short_image:"+shortName)
		for _, t := range repoTags {
			ddTags2 = append(ddTags2, "image_tag:"+t)
		}

		p.queue <- &model.ContainerImage{
			Id:          id,
			DdTags:      ddTags2,
			Name:        repo,
			Registry:    registry,
			ShortName:   shortName,
			RepoTags:    repoTags,
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
