// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	model "github.com/DataDog/agent-payload/v5/sbom"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var /* const */ (
	envVarEnv   = ddConfig.Datadog.GetString("env")
	sourceAgent = "agent"
)

type processor struct {
	queue                 chan *model.SBOMEntity
	workloadmetaStore     workloadmeta.Store
	imageRepoDigests      map[string]string              // Map where keys are image repo digest and values are image ID
	imageUsers            map[string]map[string]struct{} // Map where keys are image repo digest and values are set of container IDs
	sbomScanner           *sbomscanner.Scanner
	hostSBOM              bool
	hostScanOpts          sbom.ScanOptions
	hostname              string
	hostCache             string
	hostLastFullSBOM      time.Time
	hostHeartbeatValidity time.Duration
}

func newProcessor(workloadmetaStore workloadmeta.Store, sender sender.Sender, maxNbItem int, maxRetentionTime time.Duration, hostSBOM bool, hostHeartbeatValidity time.Duration) (*processor, error) {
	hostScanOpts := sbom.ScanOptionsFromConfig(ddConfig.Datadog, false)
	hostScanOpts.NoCache = true
	sbomScanner := sbomscanner.GetGlobalScanner()
	if sbomScanner == nil {
		return nil, errors.New("failed to get global SBOM scanner")
	}
	hostname, _ := utils.GetHostname()

	return &processor{
		queue: queue.NewQueue(maxNbItem, maxRetentionTime, func(entities []*model.SBOMEntity) {
			encoded, err := proto.Marshal(&model.SBOMPayload{
				Version:  1,
				Source:   &sourceAgent,
				Entities: entities,
				DdEnv:    &envVarEnv,
			})
			if err != nil {
				log.Errorf("Unable to encode message: %+v", err)
				return
			}

			sender.EventPlatformEvent(encoded, epforwarder.EventTypeContainerSBOM)
		}),
		workloadmetaStore:     workloadmetaStore,
		imageRepoDigests:      make(map[string]string),
		imageUsers:            make(map[string]map[string]struct{}),
		sbomScanner:           sbomScanner,
		hostSBOM:              hostSBOM,
		hostScanOpts:          hostScanOpts,
		hostname:              hostname,
		hostHeartbeatValidity: hostHeartbeatValidity,
	}, nil
}

func (p *processor) processContainerImagesEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		switch event.Entity.GetID().Kind {
		case workloadmeta.KindContainerImageMetadata:
			switch event.Type {
			case workloadmeta.EventTypeSet:
				p.registerImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
				p.processImageSBOM(event.Entity.(*workloadmeta.ContainerImageMetadata))
			case workloadmeta.EventTypeUnset:
				p.unregisterImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
				// Let the SBOM expire on back-end side
			}
		case workloadmeta.KindContainer:
			switch event.Type {
			case workloadmeta.EventTypeSet:
				p.registerContainer(event.Entity.(*workloadmeta.Container))
			case workloadmeta.EventTypeUnset:
				p.unregisterContainer(event.Entity.(*workloadmeta.Container))
			}
		}
	}
}

func (p *processor) registerImage(img *workloadmeta.ContainerImageMetadata) {
	for _, repoDigest := range img.RepoDigests {
		p.imageRepoDigests[repoDigest] = img.ID
	}
}

func (p *processor) unregisterImage(img *workloadmeta.ContainerImageMetadata) {
	for _, repoDigest := range img.RepoDigests {
		delete(p.imageUsers, repoDigest)
		if p.imageRepoDigests[repoDigest] == img.ID {
			delete(p.imageRepoDigests, repoDigest)
		}
	}
}

func (p *processor) registerContainer(ctr *workloadmeta.Container) {
	imgID := ctr.Image.ID
	ctrID := ctr.ID

	if !ctr.State.Running {
		return
	}

	if _, found := p.imageUsers[imgID]; found {
		p.imageUsers[imgID][ctrID] = struct{}{}
	} else {
		p.imageUsers[imgID] = map[string]struct{}{
			ctrID: {},
		}

		if realImgID, found := p.imageRepoDigests[imgID]; found {
			imgID = realImgID
		}

		if img, err := p.workloadmetaStore.GetImage(imgID); err != nil {
			log.Infof("Couldn’t find image %s in workloadmeta whereas it’s used by container %s: %v", imgID, ctrID, err)
		} else {
			p.processImageSBOM(img)
		}
	}
}

func (p *processor) unregisterContainer(ctr *workloadmeta.Container) {
	imgID := ctr.Image.ID
	ctrID := ctr.ID

	delete(p.imageUsers[imgID], ctrID)
	if len(p.imageUsers[imgID]) == 0 {
		delete(p.imageUsers, imgID)
	}
}

func (p *processor) processContainerImagesRefresh(allImages []*workloadmeta.ContainerImageMetadata) {
	// So far, the check is refreshing all the images every 5 minutes all together.
	for _, img := range allImages {
		p.processImageSBOM(img)
	}
}

func (p *processor) processHostRefresh() {
	if !p.hostSBOM {
		return
	}

	log.Debugf("Triggering host SBOM refresh")

	ch := make(chan sbom.ScanResult, 1)
	scanRequest := &host.ScanRequest{Path: "/"}
	if hostRoot := os.Getenv("HOST_ROOT"); config.IsContainerized() && hostRoot != "" {
		scanRequest.Path = hostRoot
	}

	if err := p.sbomScanner.Scan(scanRequest, p.hostScanOpts, ch); err != nil {
		log.Errorf("Failed to trigger SBOM generation for host: %s", err)
		return
	}

	go func() {
		result := <-ch
		log.Debugf("processing host scanresult: %v", result)

		sbom := &model.SBOMEntity{
			Status:             model.SBOMStatus_SUCCESS,
			Type:               model.SBOMSourceType_HOST_FILE_SYSTEM,
			Id:                 p.hostname,
			InUse:              true,
			GeneratedAt:        timestamppb.New(result.CreatedAt),
			GenerationDuration: convertDuration(result.Duration),
			Hash:               result.Report.ID(),
		}

		if result.Error != nil {
			sbom.Sbom = &model.SBOMEntity_Error{
				Error: result.Error.Error(),
			}
			sbom.Status = model.SBOMStatus_FAILED
		} else {
			log.Infof("Successfully generated SBOM for host: %v, %v", result.CreatedAt, result.Duration)

			if p.hostCache != "" && p.hostCache == result.Report.ID() && result.CreatedAt.Sub(p.hostLastFullSBOM) < p.hostHeartbeatValidity {
				sbom.Heartbeat = true
			} else {
				report, err := result.Report.ToCycloneDX()
				if err != nil {
					log.Errorf("Failed to extract SBOM from report: %s", err)
					sbom.Sbom = &model.SBOMEntity_Error{
						Error: err.Error(),
					}
					sbom.Status = model.SBOMStatus_FAILED
				} else {
					sbom.Sbom = &model.SBOMEntity_Cyclonedx{
						Cyclonedx: convertBOM(report),
					}
				}

				p.hostCache = result.Report.ID()
				p.hostLastFullSBOM = result.CreatedAt
			}
		}

		p.queue <- sbom
	}()
}

func (p *processor) processImageSBOM(img *workloadmeta.ContainerImageMetadata) {
	if img.SBOM == nil {
		return
	}

	if img.SBOM.Status == workloadmeta.Success && img.SBOM.CycloneDXBOM == nil {
		log.Debug("received a sbom with incorrect status")
		return
	}

	ddTags, err := tagger.Tag("container_image_metadata://"+img.ID, collectors.HighCardinality)
	if err != nil {
		log.Errorf("Could not retrieve tags for container image %s: %v", img.ID, err)
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

	inUse := false
	for _, repoDigest := range img.RepoDigests {
		if _, found := p.imageUsers[repoDigest]; found {
			inUse = true
			break
		}
	}

	for repo := range repos {
		repoSplitted := strings.Split(repo, "/")
		shortName := repoSplitted[len(repoSplitted)-1]

		id := repo + "@" + img.ID

		repoTags := make([]string, 0, len(img.RepoTags))
		for _, repoTag := range img.RepoTags {
			if strings.HasPrefix(repoTag, repo+":") {
				repoTags = append(repoTags, strings.SplitN(repoTag, ":", 2)[1])
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

		sbom := &model.SBOMEntity{
			Type:     model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
			Id:       id,
			DdTags:   ddTags2,
			RepoTags: repoTags,
			InUse:    inUse,
		}

		switch img.SBOM.Status {
		case workloadmeta.Pending:
			sbom.Status = model.SBOMStatus_PENDING
		case workloadmeta.Failed:
			sbom.Status = model.SBOMStatus_FAILED
			sbom.Sbom = &model.SBOMEntity_Error{
				Error: img.SBOM.Error,
			}
		default:
			sbom.Status = model.SBOMStatus_SUCCESS
			sbom.GeneratedAt = timestamppb.New(img.SBOM.GenerationTime)
			sbom.GenerationDuration = convertDuration(img.SBOM.GenerationDuration)
			sbom.Sbom = &model.SBOMEntity_Cyclonedx{
				Cyclonedx: convertBOM(img.SBOM.CycloneDXBOM),
			}
		}
		p.queue <- sbom
	}
}

func (p *processor) stop() {
	close(p.queue)
}
