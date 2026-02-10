// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package sbom

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/procfs"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	model "github.com/DataDog/agent-payload/v5/sbom"

	gopsutil "github.com/shirou/gopsutil/v4/host"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var /* const */ (
	sourceAgent = "agent"
)

type processor struct {
	cfg                   config.Component
	queue                 chan *model.SBOMEntity
	workloadmetaStore     workloadmeta.Component
	containerFilter       workloadfilter.FilterBundle
	tagger                tagger.Component
	imageRepoDigests      map[string]string              // Map where keys are image repo digest and values are image ID
	imageUsers            map[string]map[string]struct{} // Map where keys are image repo digest and values are set of container IDs
	sbomScanner           *sbomscanner.Scanner
	contImageSBOM         bool
	hostSBOM              bool
	procfsSBOM            bool
	hostname              string
	hostCache             string
	hostLastFullSBOM      time.Time
	hostHeartbeatValidity time.Duration
}

func newProcessor(workloadmetaStore workloadmeta.Component, filterStore workloadfilter.Component, sender sender.Sender, tagger tagger.Component, cfg config.Component, maxNbItem int, maxRetentionTime time.Duration, hostHeartbeatValidity time.Duration) (*processor, error) {
	sbomScanner := sbomscanner.GetGlobalScanner()
	if sbomScanner == nil {
		return nil, errors.New("failed to get global SBOM scanner")
	}

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting hostname: %v", err)
	}

	envVarEnv := pkgconfigsetup.Datadog().GetString("env")
	contImageSBOM := cfg.GetBool("sbom.container_image.enabled")
	hostSBOM := cfg.GetBool("sbom.host.enabled")
	procfsSBOM := isProcfsSBOMEnabled(cfg)

	return &processor{
		cfg: cfg,
		queue: queue.NewQueue(maxNbItem, maxRetentionTime, func(entities []*model.SBOMEntity) {
			encoded, err := proto.Marshal(&model.SBOMPayload{
				Version:  1,
				Host:     hname,
				Source:   &sourceAgent,
				Entities: entities,
				DdEnv:    &envVarEnv,
			})
			if err != nil {
				log.Errorf("Unable to encode message: %+v", err)
				return
			}

			sender.EventPlatformEvent(encoded, eventplatform.EventTypeContainerSBOM)
			log.Debugf("SBOM event sent with %d entities", len(entities))
		}),
		workloadmetaStore:     workloadmetaStore,
		containerFilter:       filterStore.GetContainerSBOMFilters(),
		tagger:                tagger,
		imageRepoDigests:      make(map[string]string),
		imageUsers:            make(map[string]map[string]struct{}),
		sbomScanner:           sbomScanner,
		contImageSBOM:         contImageSBOM,
		hostSBOM:              hostSBOM,
		procfsSBOM:            procfsSBOM,
		hostname:              hname,
		hostHeartbeatValidity: hostHeartbeatValidity,
	}, nil
}

func isProcfsSBOMEnabled(cfg config.Component) bool {
	// Allowed only in sidecar mode for now
	return cfg.GetBool("sbom.container.enabled") && fargate.IsSidecar()
}

func (p *processor) processContainerImagesEvents(evBundle workloadmeta.EventBundle) {
	evBundle.Acknowledge()

	log.Tracef("Processing %d events", len(evBundle.Events))

	// Separate events into images and containers
	var imageEvents []workloadmeta.Event
	var containerEvents []workloadmeta.Event

	for _, event := range evBundle.Events {
		entityID := event.Entity.GetID()
		switch entityID.Kind {
		case workloadmeta.KindContainerImageMetadata:
			imageEvents = append(imageEvents, event)
		case workloadmeta.KindContainer:
			containerEvents = append(containerEvents, event)
		}
	}

	// Process all image events first
	for _, event := range imageEvents {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			filterableContainerImage := workloadfilter.CreateContainerImage(event.Entity.(*workloadmeta.ContainerImageMetadata).Name)
			if p.containerFilter.IsExcluded(filterableContainerImage) {
				continue
			}

			p.registerImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
			p.processImageSBOM(event.Entity.(*workloadmeta.ContainerImageMetadata))
		case workloadmeta.EventTypeUnset:
			p.unregisterImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
			// Let the SBOM expire on back-end side
		}
	}

	// Process all container events after images
	for _, event := range containerEvents {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			container := event.Entity.(*workloadmeta.Container)
			p.registerContainer(container)

			filterableContainer := workloadmetafilter.CreateContainer(container, nil)
			if p.containerFilter.IsExcluded(filterableContainer) {
				continue
			}

			if p.procfsSBOM {
				if ok, err := procfs.IsAgentContainer(container.ID); !ok && err == nil {
					p.triggerProcfsScan(container)
				}
			}

			/*
				if container.SBOM != nil {
					log.Debugf("Received SBOM for running container %s with image %s (%s)", container.ID, container.Image.Name, container.Image.ID)

					if container.SBOM.CycloneDXBOM == nil {
						log.Debugf("Received empty SBOM for container %s", container.ID)
						continue
					}

					containerImage, err := p.workloadmetaStore.GetImage(container.Image.ID)
					if err != nil {
						for _, image := range p.workloadmetaStore.ListImages() {
							if image.Name == container.Image.ID || slices.Contains(image.RepoDigests, container.Image.ID) {
								containerImage = image
								break
							}
						}

						if containerImage == nil {
							log.Debugf("Failed to find image %s for container %s", container.Image.ID, container.ID)
							continue
						}
					}

					if containerImage.SBOM == nil {
						log.Debugf("Failed to find SBOM for image %s, container %s", container.Image.ID, container.ID)
						continue
					}

					imageSBOM, err := sbomutil.UncompressSBOM(containerImage.SBOM)
					if err != nil {
						log.Debugf("Failed to uncompress SBOM for image %s, container %s: %v", container.Image.ID, container.ID, err)
						continue
					}

					if imageSBOM.CycloneDXBOM.Components != nil && container.SBOM.CycloneDXBOM.Components != nil {
						imageRefs := make(map[string]bool)
						for _, component := range imageSBOM.CycloneDXBOM.Components {
							if component.Purl == nil {
								continue
							}

							imageRefs[*component.Purl] = true
						}

						i := 0
						componentCount := len(container.SBOM.CycloneDXBOM.Components)
						for _, component := range container.SBOM.CycloneDXBOM.Components {
							if component.Purl == nil {
								continue
							}

							if imageRefs[*component.Purl] {
								container.SBOM.CycloneDXBOM.Components = append((container.SBOM.CycloneDXBOM.Components)[:i], (container.SBOM.CycloneDXBOM.Components)[i+1:]...)
								continue
							}
							i++
						}

						log.Infof("Stripped %d from %d components from SBOM for container %s that were parts of the %d components of the base image %s", componentCount-len(container.SBOM.CycloneDXBOM.Components), componentCount, container.ID, len(imageSBOM.CycloneDXBOM.Components), container.Image.ID)
					}

					status := model.SBOMStatus_value[string(container.SBOM.Status)]
					sbomEntity := &model.SBOMEntity{
						Type:               model.SBOMSourceType_CONTAINER_FILE_SYSTEM,
						Id:                 container.ID,
						GeneratedAt:        timestamppb.New(container.SBOM.GenerationTime),
						GenerationDuration: convertDuration(container.SBOM.GenerationDuration),
						InUse:              true,
						Sbom: &model.SBOMEntity_Cyclonedx{
							Cyclonedx: container.SBOM.CycloneDXBOM,
						},
						Status: model.SBOMStatus(status),
					}

					p.queue <- sbomEntity
				}
			*/
		case workloadmeta.EventTypeUnset:
			p.unregisterContainer(event.Entity.(*workloadmeta.Container))
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

func (p *processor) processHostScanResult(result sbom.ScanResult) {
	log.Debugf("processing host scanresult: %v", result)

	info, err := gopsutil.Info()
	if err != nil {
		log.Warnf("Failed to get host info: %v", err)
		info = &gopsutil.InfoStat{}
	}

	sbom := &model.SBOMEntity{
		Status:             model.SBOMStatus_SUCCESS,
		Type:               model.SBOMSourceType_HOST_FILE_SYSTEM,
		Id:                 p.hostname,
		InUse:              true,
		GeneratedAt:        timestamppb.New(result.CreatedAt),
		GenerationDuration: bomconvert.ConvertDuration(result.Duration),
		CpuArchitecture:    info.KernelArch,
		KernelVersion:      info.KernelVersion,
	}

	if result.Error != nil {
		log.Errorf("Scan error: %v", result.Error)
		sbom.Sbom = &model.SBOMEntity_Error{
			Error: result.Error.Error(),
		}
		sbom.Status = model.SBOMStatus_FAILED
	} else {
		log.Infof("Successfully generated SBOM for host: %v, %v", result.CreatedAt, result.Duration)

		if p.hostCache != "" && p.hostCache == result.Report.ID() && result.CreatedAt.Sub(p.hostLastFullSBOM) < p.hostHeartbeatValidity {
			sbom.Heartbeat = true
		} else {
			report := result.Report.ToCycloneDX()
			sbom.Sbom = &model.SBOMEntity_Cyclonedx{
				Cyclonedx: report,
			}

			sbom.Hash = result.Report.ID()
			p.hostCache = result.Report.ID()
			p.hostLastFullSBOM = result.CreatedAt
		}
	}

	p.queue <- sbom
}

func convertDuration(in time.Duration) *durationpb.Duration {
	return durationpb.New(in)
}

func (p *processor) triggerHostScan() {
	if !p.hostSBOM {
		return
	}
	log.Debugf("Triggering host SBOM refresh")

	scanRequest := host.NewHostScanRequest()

	if err := p.sbomScanner.Scan(scanRequest); err != nil {
		log.Errorf("Failed to trigger SBOM generation for host: %s", err)
		return
	}
}

func (p *processor) triggerProcfsScan(ctr *workloadmeta.Container) {
	log.Debugf("Triggering procfs SBOM scan : %s", ctr.ID)

	scanRequest := procfs.NewScanRequest(ctr.ID)
	if err := p.sbomScanner.Scan(scanRequest); err != nil {
		log.Errorf("Failed to trigger SBOM generation for procfs: %s", err)
	}
}

func (p *processor) processProcfsScanResult(result sbom.ScanResult) {
	log.Debugf("processing procfs scanresult: %v", result)

	info, err := gopsutil.Info()
	if err != nil {
		log.Warnf("Failed to get host info: %v", err)
		info = &gopsutil.InfoStat{}
	}

	sbom := &model.SBOMEntity{
		Status:             model.SBOMStatus_SUCCESS,
		Id:                 result.RequestID,
		Type:               model.SBOMSourceType_CONTAINER_FILE_SYSTEM,
		InUse:              true,
		GeneratedAt:        timestamppb.New(result.CreatedAt),
		GenerationDuration: bomconvert.ConvertDuration(result.Duration),
		CpuArchitecture:    info.KernelArch,
		KernelVersion:      info.KernelVersion,
	}

	if result.Error != nil {
		if result.Error == procfs.ErrNotFound {
			return
		}

		log.Errorf("Scan error: %v", result.Error)
		sbom.Sbom = &model.SBOMEntity_Error{
			Error: result.Error.Error(),
		}
		sbom.Status = model.SBOMStatus_FAILED
	} else {
		log.Infof("Successfully generated SBOM for procfs: %v, %v", result.CreatedAt, result.Duration)
		if p.hostCache != "" && p.hostCache == result.Report.ID() && result.CreatedAt.Sub(p.hostLastFullSBOM) < p.hostHeartbeatValidity {
			sbom.Heartbeat = true
		} else {
			report := result.Report.ToCycloneDX()
			sbom.Sbom = &model.SBOMEntity_Cyclonedx{
				Cyclonedx: report,
			}
		}
	}

	p.queue <- sbom
}

func (p *processor) processImageSBOM(img *workloadmeta.ContainerImageMetadata) {
	if !p.contImageSBOM {
		return
	}

	if img.SBOM == nil {
		return
	}

	if img.SBOM.Status == workloadmeta.Success && len(img.SBOM.Bom) == 0 {
		log.Debug("received a sbom with incorrect status")
		return
	}

	entityID := types.NewEntityID(types.ContainerImageMetadata, img.ID)
	ddTags, err := p.tagger.Tag(entityID, types.HighCardinality)
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

	cyclosbom, err := sbomutil.UncompressSBOM(img.SBOM)
	if err != nil {
		log.Errorf("Failed to uncompress SBOM for image %s: %v", img.ID, err)
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

		repoDigests := make([]string, 0, len(img.RepoDigests))
		for _, repoDigest := range img.RepoDigests {
			if strings.HasPrefix(repoDigest, repo+"@sha256:") {
				repoDigests = append(repoDigests, repoDigest)
			}
		}

		if len(repoDigests) == 0 {
			allowMissingRepodigest := p.cfg.GetBool("sbom.container_image.allow_missing_repodigest")
			if !allowMissingRepodigest || len(img.RepoDigests) != 0 {
				log.Infof("The image %s has no repo digest for repo %s, skipping", img.ID, repo)
				continue
			}

			if !inUse {
				_, inUse = p.imageUsers[img.ID]
			}

			log.Infof("The image %s has no repo digest for repo %s", img.Name, repo)
		}

		// Because we split a single image entity into different payloads if it has several repo digests,
		// we must re-compute `image_id`, `image_name`, `short_image` and `image_tag` tags.
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

		if img.SBOM.GenerationMethod != "" {
			ddTags2 = append(ddTags2, sbom.ScanMethodTagName+":"+img.SBOM.GenerationMethod)
		}

		sbom := &model.SBOMEntity{
			Type:        model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
			Id:          id,
			DdTags:      ddTags2,
			RepoTags:    repoTags,
			RepoDigests: repoDigests,
			InUse:       inUse,
		}

		switch cyclosbom.Status {
		case workloadmeta.Pending:
			sbom.Status = model.SBOMStatus_PENDING
		case workloadmeta.Failed:
			sbom.Status = model.SBOMStatus_FAILED
			sbom.Sbom = &model.SBOMEntity_Error{
				Error: cyclosbom.Error,
			}
		default:
			sbom.Status = model.SBOMStatus_SUCCESS
			sbom.GeneratedAt = timestamppb.New(cyclosbom.GenerationTime)
			sbom.GenerationDuration = bomconvert.ConvertDuration(cyclosbom.GenerationDuration)
			sbom.Sbom = &model.SBOMEntity_Cyclonedx{
				Cyclonedx: cyclosbom.CycloneDXBOM,
			}
		}
		p.queue <- sbom
	}
}

func (p *processor) stop() {
	close(p.queue)
}
