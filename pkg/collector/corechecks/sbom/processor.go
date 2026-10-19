// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || windows

package sbom

import (
	"context"
	"errors"
	"strconv"
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
	sbomtelemetry "github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"
	pkgimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	model "github.com/DataDog/agent-payload/v5/sbom"

	gopsutil "github.com/shirou/gopsutil/v4/host"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var /* const */ (
	sourceAgent = "agent"
)

// usageGracePeriod bounds how long the first SBOM of a workload waits for the
// runtime usage system-probe reports, so that the SBOM reaches the back end
// once, already carrying its usage.
const usageGracePeriod = 2 * time.Minute

// scanRequester queues SBOM scan requests, as the global scanner does.
type scanRequester interface {
	Scan(sbom.ScanRequest) error
}

type processor struct {
	cfg                   config.Component
	queue                 chan *model.SBOMEntity
	workloadmetaStore     workloadmeta.Component
	containerFilter       workloadfilter.FilterBundle
	tagger                tagger.Component
	imageRepoDigests      map[string]string   // Map where keys are image repo digest and values are image ID
	imagesInUse           map[string]struct{} // Set of image IDs the back end was last told are in use
	sbomScanner           scanRequester
	contImageSBOM         bool
	hostSBOM              bool
	procfsSBOM            bool
	hostname              string
	hostCache             string
	hostLastFullSBOM      time.Time
	hostHeartbeatValidity time.Duration
	hostUsage             *cyclonedx_v1_4.Bom // Latest runtime usage report of the host, merged into the host scans

	// The first SBOM of a workload waits for its runtime usage when
	// usageEnrichment is on, for usageGracePeriod at most.
	usageEnrichment      bool
	heldImages           map[string]time.Time // Images whose first SBOM waits for usage, with the time it was first held
	imagesWaited         map[string]struct{}  // Images whose SBOM went out in use, done waiting for usage
	hostHeld             *sbom.ScanResult     // First host scan result, waiting for usage
	hostHeldSince        time.Time
	hostSentWithoutUsage bool // The host SBOM went out before the first usage report, which then triggers a scan

	// While the usage of a workload was recorded for less than its window, the
	// unseen packages go out stripped of their runtime properties, which leaves
	// their usage unknown.
	imageUsageWindow   time.Duration
	hostUsageWindow    time.Duration
	hostSentWindowOpen bool // The last full host SBOM went out while the window of the host was open

	clock func() time.Time // Returns the current time, time.Now when nil
}

// now returns the current time, as clock tells it.
func (p *processor) now() time.Time {
	if p.clock == nil {
		return time.Now()
	}
	return p.clock()
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

			for _, entity := range entities {
				sbomtelemetry.SBOMEntitiesSent.Inc(
					strings.ToLower(entity.Type.String()),
					strings.ToLower(entity.Status.String()),
					strconv.FormatBool(entity.Heartbeat),
					strconv.FormatBool(sbom.IsEnriched(entity.GetCyclonedx())),
				)
			}
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
		usageEnrichment:       sbom.UsageEnrichmentEnabled(cfg),
		heldImages:            make(map[string]time.Time),
		imagesWaited:          make(map[string]struct{}),
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

	delete(p.heldImages, img.ID)
	delete(p.imagesWaited, img.ID)
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

	if p.hostWaitsForUsage(result, p.now()) {
		log.Debugf("The host SBOM waits for the runtime usage of the host")
		return
	}

	p.sendHostScanResult(result)
}

// hostWaitsForUsage reports whether the host scan result waits for the runtime
// usage of the host before it goes out, as the first successful one does until
// system-probe reports the usage, for usageGracePeriod at most. A newer result
// replaces the one held and keeps its deadline.
func (p *processor) hostWaitsForUsage(result sbom.ScanResult, now time.Time) bool {
	if !p.usageEnrichment || result.Error != nil || p.hostUsage != nil || !p.hostLastFullSBOM.IsZero() {
		return false
	}

	if p.hostHeld == nil {
		p.hostHeldSince = now
	} else if now.Sub(p.hostHeldSince) >= usageGracePeriod {
		p.hostHeld = nil
		return false
	}

	p.hostHeld = &result
	return true
}

// sendHostScanResult sends the host SBOM of result, in full or as a heartbeat.
func (p *processor) sendHostScanResult(result sbom.ScanResult) {
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

		// The SBOM the back end holds hides the unseen packages while the window
		// is open, so the window closing makes it stale.
		windowOpen := sbomutil.UsageWindowOpen(p.hostUsage, p.now(), p.hostUsageWindow)

		if p.hostCache != "" && p.hostCache == result.Report.ID() && result.CreatedAt.Sub(p.hostLastFullSBOM) < p.hostHeartbeatValidity && windowOpen == p.hostSentWindowOpen {
			sbom.Heartbeat = true
		} else {
			report := sbomutil.MergeRuntimeProperties(result.Report.ToCycloneDX(), p.hostUsage)
			sbomutil.HideUnreported(report, p.hostUsage)
			if windowOpen {
				sbomutil.HideUnobserved(report)
			}
			sbom.Sbom = &model.SBOMEntity_Cyclonedx{
				Cyclonedx: report,
			}

			sbom.Hash = result.Report.ID()
			p.hostCache = result.Report.ID()
			p.hostLastFullSBOM = result.CreatedAt
			p.hostSentWithoutUsage = p.usageEnrichment && p.hostUsage == nil
			p.hostSentWindowOpen = windowOpen
		}
	}

	p.queue <- sbom
}

// processHostUsage records usage, the runtime usage report system-probe
// forwards for the host, for the host scans to carry. The host SBOM the back
// end holds predates usage, so the next scan goes out in full. A host SBOM
// waiting for its first usage goes out now, and when the host SBOM went out
// before the first report, a scan carries that report right away.
func (p *processor) processHostUsage(usage *cyclonedx_v1_4.Bom) {
	log.Debugf("processing host runtime usage report of %d packages", len(usage.GetComponents()))

	p.hostUsage = usage
	p.hostCache = ""

	if held := p.hostHeld; held != nil {
		p.hostHeld = nil
		p.sendHostScanResult(*held)
		return
	}

	if p.hostSentWithoutUsage {
		p.hostSentWithoutUsage = false
		p.triggerHostScan()
	}
}

// releaseExpiredHolds sends the SBOMs that waited usageGracePeriod for their
// runtime usage as they are.
func (p *processor) releaseExpiredHolds(now time.Time) {
	if p.hostHeld != nil && now.Sub(p.hostHeldSince) >= usageGracePeriod {
		held := p.hostHeld
		p.hostHeld = nil
		log.Infof("The host SBOM goes out without runtime usage after waiting %s for it", usageGracePeriod)
		p.sendHostScanResult(*held)
	}

	var expired []string
	for imgID, since := range p.heldImages {
		if now.Sub(since) >= usageGracePeriod {
			expired = append(expired, imgID)
		}
	}
	if len(expired) == 0 {
		return
	}

	running := runningImages(p.workloadmetaStore)
	for _, imgID := range expired {
		delete(p.heldImages, imgID)

		img, err := p.workloadmetaStore.GetImage(imgID)
		if err != nil {
			continue
		}
		// An image in use goes out past its wait. One left unused waits again
		// for its next container.
		if imageInUse(img, running) {
			p.imagesWaited[imgID] = struct{}{}
		}
		log.Infof("The SBOM of image %s goes out without runtime usage after waiting %s for it", imgID, usageGracePeriod)
		p.processImageSBOM(img, running)
	}
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

	if p.imageWaitsForUsage(img.ID, cyclosbom, inUse, p.now()) {
		log.Debugf("The SBOM of image %s waits for the runtime usage of the image", img.ID)
		return
	}

	// UncompressSBOM returns a BOM of its own, which every repo entity shares.
	if cyclosbom.Status == workloadmeta.Success && sbomutil.UsageWindowOpen(cyclosbom.CycloneDXBOM, p.now(), p.imageUsageWindow) {
		sbomutil.HideUnobserved(cyclosbom.CycloneDXBOM)
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

	// An SBOM sent before the first container of the image runs leaves the
	// image waiting, as the usage comes with that container.
	if p.usageEnrichment && cyclosbom.Status == workloadmeta.Success {
		delete(p.heldImages, img.ID)
		if inUse {
			p.imagesWaited[img.ID] = struct{}{}
		}
	}
}

// imageWaitsForUsage reports whether the SBOM of the image imgID waits for the
// runtime usage of the image before it goes out, as the first successful SBOM
// of an image in use does until its usage merges, for usageGracePeriod at most,
// counted from the first time it waited. Usage covers the OS packages alone, so
// the SBOM waits when it lists one.
func (p *processor) imageWaitsForUsage(imgID string, s *workloadmeta.SBOM, inUse bool, now time.Time) bool {
	if !p.usageEnrichment || !inUse || s.Status != workloadmeta.Success {
		return false
	}

	if _, waited := p.imagesWaited[imgID]; waited {
		return false
	}

	if sbom.IsEnriched(s.CycloneDXBOM) || !sbomutil.HasOSPackage(s.CycloneDXBOM) {
		return false
	}

	since, held := p.heldImages[imgID]
	if !held {
		p.heldImages[imgID] = now
		return true
	}

	return now.Sub(since) < usageGracePeriod
}

func (p *processor) stop() {
	close(p.queue)
}
