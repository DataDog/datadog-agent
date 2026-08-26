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
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/procfs"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	pkgimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	model "github.com/DataDog/agent-payload/v5/sbom"

	gopsutil "github.com/shirou/gopsutil/v4/host"
	"google.golang.org/protobuf/proto"
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
	imageRepoDigests      map[string]string   // Map where keys are image repo digest and values are image ID
	imagesInUse           map[string]struct{} // Set of image IDs the back end was last told are in use
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
		imagesInUse:           make(map[string]struct{}),
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
	// The store already reflects the events in this bundle, so ask it which
	// images are in use rather than tracking container events ourselves. Ask
	// before acknowledging: the store hands the next bundle to the next
	// subscriber as soon as this one acknowledges, and would then answer for a
	// later moment than the bundle being processed describes.
	running := runningImages(p.workloadmetaStore)

	evBundle.Acknowledge()

	log.Tracef("Processing %d events", len(evBundle.Events))

	// Separate events by kind and type. Image events are handled first so that
	// imageRepoDigests is up to date when the identifiers the containers use
	// are resolved below.
	var (
		imageSetEvents     []workloadmeta.Event
		imageUnsetEvents   []workloadmeta.Event
		containerSetEvents []workloadmeta.Event
	)

	for _, event := range evBundle.Events {
		switch event.Entity.GetID().Kind {
		case workloadmeta.KindContainerImageMetadata:
			if event.Type == workloadmeta.EventTypeSet {
				imageSetEvents = append(imageSetEvents, event)
			} else {
				imageUnsetEvents = append(imageUnsetEvents, event)
			}
		case workloadmeta.KindContainer:
			if event.Type == workloadmeta.EventTypeSet {
				containerSetEvents = append(containerSetEvents, event)
			}
		}
	}

	// Images reported in this bundle, so that an image and the container that
	// just started it don't each produce an SBOM.
	reported := make(map[string]struct{}, len(imageSetEvents))

	for _, event := range imageUnsetEvents {
		p.unregisterImage(event.Entity.(*workloadmeta.ContainerImageMetadata))
		// Let the SBOM expire on back-end side
	}

	for _, event := range imageSetEvents {
		img := event.Entity.(*workloadmeta.ContainerImageMetadata)

		filterableContainerImage := workloadfilter.CreateContainerImage(img.Name)
		if p.containerFilter.IsExcluded(filterableContainerImage) {
			continue
		}

		p.registerImage(img)
		p.processImageSBOM(img, running)
		reported[img.ID] = struct{}{}
	}

	// Report images that gained their first running container, so that the
	// back end learns about them without waiting for the periodic refresh.
	// Containers name the same image in more than one way, so compare resolved
	// image IDs rather than the identifiers they use.
	imagesInUse := make(map[string]struct{}, len(running))
	for id := range running {
		imgID := p.resolveImageID(id)
		imagesInUse[imgID] = struct{}{}

		if _, found := p.imagesInUse[imgID]; !found {
			p.reportImage(imgID, running, reported)
		}
	}
	p.imagesInUse = imagesInUse

	for _, event := range containerSetEvents {
		container := event.Entity.(*workloadmeta.Container)

		filterableContainer := workloadmetafilter.CreateContainer(container, nil)
		if p.containerFilter.IsExcluded(filterableContainer) {
			continue
		}

		if p.procfsSBOM {
			if ok, err := procfs.IsAgentContainer(container.ID); !ok && err == nil {
				p.triggerProcfsScan(container)
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
		if p.imageRepoDigests[repoDigest] == img.ID {
			delete(p.imageRepoDigests, repoDigest)
		}
	}
}

// runningImages returns the identifiers of the images that have at least one
// running container. Depending on the runtime and on which workloadmeta
// sources describe it, a container names its image either by image ID or by
// repo digest, so the identifiers are returned as the containers spell them.
func runningImages(store workloadmeta.Component) map[string]struct{} {
	containers := store.ListContainersWithFilter(workloadmeta.GetRunningContainers)

	images := make(map[string]struct{}, len(containers))
	for _, ctr := range containers {
		if ctr.Image.ID != "" {
			images[ctr.Image.ID] = struct{}{}
		}
	}

	return images
}

// imageInUse reports whether one of the running images is img, named either by
// its ID or by one of its repo digests.
func imageInUse(img *workloadmeta.ContainerImageMetadata, running map[string]struct{}) bool {
	if _, found := running[img.ID]; found {
		return true
	}

	for _, repoDigest := range img.RepoDigests {
		if _, found := running[repoDigest]; found {
			return true
		}
	}

	return false
}

// resolveImageID maps the identifier a container uses to name its image to the
// ID of the corresponding image entity. Identifiers that are already image IDs
// are returned unchanged.
func (p *processor) resolveImageID(id string) string {
	if imgID, found := p.imageRepoDigests[id]; found {
		return imgID
	}

	return id
}

// reportImage emits the SBOM of the image named by imgID, unless it has already
// been reported for the event bundle being processed.
func (p *processor) reportImage(imgID string, running, reported map[string]struct{}) {
	if _, found := reported[imgID]; found {
		return
	}

	img, err := p.workloadmetaStore.GetImage(imgID)
	if err != nil {
		log.Infof("Couldn't find image %s in workloadmeta although a container runs it: %v", imgID, err)
		return
	}

	p.processImageSBOM(img, running)
	reported[imgID] = struct{}{}
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

func (p *processor) processImageSBOM(img *workloadmeta.ContainerImageMetadata, running map[string]struct{}) {
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
		// Split on the last colon (after the last slash) so registries that
		// include a port are parsed correctly.
		repoName, _ := pkgimage.SplitRepoTag(repoTag)
		repos[repoName] = struct{}{}
	}

	inUse := imageInUse(img, running)
	if !inUse {
		// A periodic refresh reaches this with no event bundle behind it, so
		// forget the image here rather than only when a bundle rebuilds the
		// set. Otherwise the back end is told the image is not in use while
		// the set still says it is, and a container starting it again is
		// taken for one that changes nothing and goes unreported.
		delete(p.imagesInUse, img.ID)
	}

	cyclosbom, err := sbomutil.UncompressSBOM(img.SBOM)
	if err != nil {
		log.Errorf("Failed to uncompress SBOM for image %s: %v", img.ID, err)
		return
	}

	for repo := range repos {
		repoSplitted := strings.Split(repo, "/")
		shortName := repoSplitted[len(repoSplitted)-1]

		id := repo + "@" + img.ID

		repoTags := make([]string, 0, len(img.RepoTags))
		for _, repoTag := range img.RepoTags {
			repoName, tag := pkgimage.SplitRepoTag(repoTag)
			if repoName == repo && tag != "" {
				repoTags = append(repoTags, tag)
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
