// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sbom holds sbom related files
package sbom

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/samber/lo"
	"github.com/skydive-project/go-debouncer"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/collectorv2"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// state of the sboms
	pendingState int64 = iota + 1
	computedState
	stoppedState

	maxSBOMGenerationRetries = 3
	maxSBOMEntries           = 1024
	scanQueueSize            = 100
)

var errNoProcessForContainerID = errors.New("found no running process matching the given container ID")

// Event defines the SBOM event type
type Event int

const (
	// SBOMComputed is used to notify that a SBOM was computed
	SBOMComputed Event = iota + 1
)

// Data use the keep the result of a scan of a same workload across multiple
// container
type Data struct {
	files fileQuerier
}

// SBOM defines an SBOM
type SBOM struct {
	sync.RWMutex

	ContainerID containerutils.ContainerID

	data *Data

	workloadKey workloadKey

	cgroup *cgroupModel.CacheEntry
	state  *atomic.Int64

	refresher *debouncer.Debouncer
}

type workloadKey string

func getWorkloadKey(selector *cgroupModel.WorkloadSelector) workloadKey {
	return workloadKey(selector.Image + ":" + selector.Tag)
}

// IsComputed returns true if SBOM was successfully generated
func (s *SBOM) IsComputed() bool {
	return s.state.Load() == computedState
}

// SetReport sets the SBOM report
func (s *SBOM) setReport(pkgs []sbomtypes.PackageWithInstalledFiles) {
	// build file cache
	s.data.files = newFileQuerier(pkgs)
}

func (s *SBOM) stop() {
	if s.refresher != nil {
		s.refresher.Stop()

		// don't forget to set the refresher to nil otherwise it generates a memleak
		s.refresher = nil
	}

	// change the state so that already queued sbom won't be handled
	s.state.Store(stoppedState)
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	return &SBOM{
		ContainerID: id,
		workloadKey: workloadKey,
		state:       atomic.NewInt64(pendingState),
		cgroup:      cgroup,
		data:        &Data{},
	}
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	*utils.Notifier[Event, *sbom.ScanResult]

	cfg *config.RuntimeSecurityConfig

	sbomsLock sync.RWMutex
	sboms     *simplelru.LRU[containerutils.ContainerID, *SBOM]

	// cache
	dataCacheLock sync.RWMutex
	dataCache     *simplelru.LRU[workloadKey, *Data] // cache per workload key

	// queue
	scanChan        chan *SBOM
	pendingScanLock sync.Mutex
	pendingScan     []containerutils.ContainerID

	statsdClient   statsd.ClientInterface
	sbomCollector  sbomCollector
	hostRootDevice uint64
	hostSBOM       *SBOM

	sbomGenerations       *atomic.Uint64
	failedSBOMGenerations *atomic.Uint64
	sbomsCacheHit         *atomic.Uint64
	sbomsCacheMiss        *atomic.Uint64

	wmeta workloadmeta.Component

	// Callback for when SBOM policies should be generated
	policyGeneratorCb func(workloadKey string, containerID containerutils.ContainerID, policyDef *rules.PolicyDef)
	policyGenLock     sync.RWMutex
}

type sbomCollector interface {
	ScanInstalledPackages(ctx context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error)
}

// NewSBOMResolver returns a new instance of Resolver
func NewSBOMResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, wmeta workloadmeta.Component) (*Resolver, error) {
	sbomCollector := collectorv2.NewOSScanner()
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](c.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	hostProcRootPath := utils.ProcRootPath(1)
	stat, err := utils.UnixStat(hostProcRootPath)
	if err != nil {
		return nil, fmt.Errorf("stat failed for `%s`: couldn't stat host proc root path: %w", hostProcRootPath, err)
	}

	resolver := &Resolver{
		Notifier:              utils.NewNotifier[Event, *sbom.ScanResult](),
		cfg:                   c,
		statsdClient:          statsdClient,
		dataCache:             dataCache,
		scanChan:              make(chan *SBOM, 100),
		sbomCollector:         sbomCollector,
		hostRootDevice:        stat.Dev,
		sbomGenerations:       atomic.NewUint64(0),
		sbomsCacheHit:         atomic.NewUint64(0),
		sbomsCacheMiss:        atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
		wmeta:                 wmeta,
	}

	sboms, err := simplelru.NewLRU(maxSBOMEntries, func(_ containerutils.ContainerID, sbom *SBOM) {
		// should be trigger from a function already locking the sbom, see Add, Delete
		sbom.stop()
		resolver.removePendingScan(sbom.ContainerID)
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOM resolver: %w", err)
	}
	resolver.sboms = sboms

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	return resolver, nil
}

// Start starts the goroutine of the SBOM resolver
func (r *Resolver) Start(ctx context.Context) error {
	if r.cfg.SBOMResolverHostEnabled {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/"
		}

		r.hostSBOM = NewSBOM("", nil, "")

		report, err := r.generateSBOM(hostRoot)
		if err != nil {
			return err
		}
		r.hostSBOM.setReport(report)
		r.hostSBOM.state.Store(computedState)
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case sbom := <-r.scanChan:
				if err := retry.Do(func() error {
					return r.analyzeWorkload(sbom)
				}, retry.Attempts(maxSBOMGenerationRetries), retry.Delay(200*time.Millisecond), retry.DelayType(retry.FixedDelay)); err != nil {
					if errors.Is(err, errNoProcessForContainerID) {
						seclog.Debugf("Couldn't generate SBOM for '%s': %v", sbom.ContainerID, err)
					} else {
						seclog.Warnf("Failed to generate SBOM for '%s': %v", sbom.ContainerID, err)
					}
				}
			case <-ticker.C:
				seclog.Debugf("Enriching SBOM with runtime usage")
				if err := r.enrichSBOMsWithUsage(); err != nil {
					seclog.Errorf("Couldn't enrich SBOMs with usage: %v", err)
				}
			}
		}
	}()

	return nil
}

// RefreshSBOM regenerates a SBOM for a container
func (r *Resolver) RefreshSBOM(containerID containerutils.ContainerID) error {
	if sbom := r.getSBOM(containerID); sbom != nil {
		seclog.Debugf("Refreshing SBOM for container %s", containerID)

		var refresher *debouncer.Debouncer

		// create a refresher debouncer on demand
		sbom.Lock()
		refresher = sbom.refresher
		if refresher == nil {
			refresher = debouncer.New(
				3*time.Second, func() {
					// invalid cache data
					r.removeSBOMData(workloadKey(sbom.ContainerID))

					sbom.Lock()
					r.triggerScan(sbom)
					sbom.Unlock()
				},
			)
			refresher.Start()
			sbom.refresher = refresher
		}
		sbom.Unlock()

		refresher.Call()

		return nil
	}
	return fmt.Errorf("container %s not found", containerID)
}

// generateSBOM calls the collector to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	report, err := r.sbomCollector.ScanInstalledPackages(context.Background(), root)
	if err != nil {
		r.failedSBOMGenerations.Inc()
		return nil, fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	seclog.Infof("SBOM successfully generated from %s with %d packages", root, len(report))

	return report, nil
}

// generateSBOMPolicyDef generates a policy definition from SBOM packages
func (r *Resolver) generateSBOMPolicyDef(containerID containerutils.ContainerID, packages []sbomtypes.PackageWithInstalledFiles) *rules.PolicyDef {
	var sbomMacros []*rules.MacroDefinition
	var sbomRules []*rules.RuleDefinition
	fileCount := 0

	for _, pkg := range packages {
		if len(pkg.InstalledFiles) == 0 {
			continue
		}

		fileCount += len(pkg.InstalledFiles)
		pkgName := strings.ReplaceAll(pkg.Package.Name, "-", "_")
		pkgName = strings.ReplaceAll(pkgName, ".", "_")
		macroName := "sbom_pkg_files_" + strings.ToLower(pkgName)

		// Build the macro expression with the list of files
		macroExpression := "[ " + strings.Join(lo.Map(pkg.InstalledFiles, func(file string, i int) string {
			return `"` + file + `"`
		}), ", ") + " ]"

		sbomMacros = append(sbomMacros, &rules.MacroDefinition{
			ID:         macroName,
			Expression: macroExpression,
		})

		// Create a rule that detects when files from this package are opened
		ruleName := "sbom_detect_file_in_pkg_" + strings.ToLower(pkgName)
		sbomRules = append(sbomRules, &rules.RuleDefinition{
			ID:         ruleName,
			Expression: "open.file.path in " + macroName,
			Silent:     true, // These are internal tracking rules
			Tags: map[string]string{
				"sbom_container":       string(containerID),
				"sbom_package_name":    pkg.Package.Name,
				"sbom_package_version": pkg.Package.Version,
			},
		})

		seclog.Debugf("Generated SBOM macro: %s with %d files", macroName, len(pkg.InstalledFiles))
	}

	if len(sbomMacros) == 0 {
		return nil
	}

	seclog.Infof("Generated %d SBOM macros and %d rules for container %s (%d total files)",
		len(sbomMacros), len(sbomRules), containerID, fileCount)

	return &rules.PolicyDef{
		Macros: sbomMacros,
		Rules:  sbomRules,
	}
}

func (r *Resolver) doScan(sbom *SBOM) ([]sbomtypes.PackageWithInstalledFiles, error) {
	var (
		lastErr error
		scanned bool
		report  []sbomtypes.PackageWithInstalledFiles
	)

	cfs := utils.DefaultCGroupFS()

	for _, rootCandidatePID := range sbom.cgroup.GetPIDs() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, _, _, err := cfs.FindCGroupContext(rootCandidatePID, rootCandidatePID)
		if err != nil {
			continue
		}
		if computedID != sbom.ContainerID {
			continue
		}

		containerProcRootPath := utils.ProcRootPath(rootCandidatePID)
		if sbom.ContainerID != "" {
			stat, err := utils.UnixStat(containerProcRootPath)
			if err != nil {
				return nil, fmt.Errorf("stat failed for `%s`: couldn't stat container proc root path: %w", containerProcRootPath, err)
			}
			if stat.Dev == r.hostRootDevice {
				return nil, fmt.Errorf("couldn't generate sbom: filesystem of container '%s' matches the host root filesystem", sbom.ContainerID)
			}
		}

		if report, lastErr = r.generateSBOM(containerProcRootPath); lastErr == nil {
			sbom.setReport(report)
			scanned = true
			break
		}

		seclog.Errorf("couldn't generate SBOM: %v", lastErr)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if !scanned {
		return nil, errNoProcessForContainerID
	}
	return report, nil
}

func (r *Resolver) removeSBOMData(key workloadKey) {
	r.dataCacheLock.Lock()
	r.dataCache.Remove(key)
	r.dataCacheLock.Unlock()
}

func (r *Resolver) enrichSBOMsWithUsage() error {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	images := r.wmeta.ListImages()

	for _, image := range images {
		uncompressedSBOM, err := sbomutil.UncompressSBOM(image.SBOM)
		if err != nil {
			return err
		}

		for _, sbom := range r.sboms.Values() {
			if sbom.data != nil && sbom.workloadKey == workloadKey(image.Name) {
				r.enrichSBOMWithUsage(uncompressedSBOM, sbom)
			}
		}
	}

	return nil
}

func (r *Resolver) enrichSBOMWithUsage(wsbom *workloadmeta.SBOM, sbom *SBOM) {
	componentMap := make(map[string]*cyclonedx_v1_4.Component)
	for _, component := range wsbom.CycloneDXBOM.Components {
		componentMap[component.Name+"/"+component.Version] = component
	}

	files := sbom.data.files
	for _, pkg := range files.pkgs {
		if !pkg.LastAccess.IsZero() {
			if component, ok := componentMap[pkg.Name+"/"+pkg.Version]; ok {
				if component.Properties == nil {
					component.Properties = []*cyclonedx_v1_4.Property{}
				}

				lastAccess := pkg.LastAccess.Format(time.RFC3339)
				component.Properties = append(component.Properties, &cyclonedx_v1_4.Property{
					Name:  "LastAccess",
					Value: &lastAccess,
				})
			}
		}
	}
}

func (r *Resolver) addPendingScan(containerID containerutils.ContainerID) bool {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	if len(r.pendingScan) >= scanQueueSize {
		return false
	}

	if slices.Contains(r.pendingScan, containerID) {
		return false
	}
	r.pendingScan = append(r.pendingScan, containerID)

	return true
}

func (r *Resolver) removePendingScan(containerID containerutils.ContainerID) {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	r.pendingScan = slices.DeleteFunc(r.pendingScan, func(v containerutils.ContainerID) bool {
		return v == containerID
	})
}

// analyzeWorkload generates the SBOM of the provided sbom and send it to the security agent
func (r *Resolver) analyzeWorkload(sb *SBOM) error {
	sb.Lock()
	defer sb.Unlock()

	seclog.Infof("analyzing sbom '%s'", sb.ContainerID)

	if currentState := sb.state.Load(); currentState != pendingState {
		r.removePendingScan(sb.ContainerID)

		if currentState != stoppedState {
			// should not append, ignore
			seclog.Warnf("trying to analyze a sbom not in pending state for '%s': %d", sb.ContainerID, currentState)
			return nil
		}
	}

	// bail out if the workload has been analyzed while queued up
	r.dataCacheLock.RLock()
	if data, exists := r.dataCache.Get(sb.workloadKey); exists {
		r.dataCacheLock.RUnlock()
		sb.data = data

		r.removePendingScan(sb.ContainerID)

		return nil
	}
	r.dataCacheLock.RUnlock()

	report, scanErr := r.doScan(sb)
	if scanErr != nil {
		return scanErr
	}

	// r.NotifyListeners(SBOMComputed, report)

	data := &Data{
		files: newFileQuerier(report),
	}
	sb.data = data

	// mark the SBOM as successful
	sb.state.Store(computedState)

	// add to cache
	r.dataCacheLock.Lock()
	r.dataCache.Add(workloadKey(sb.ContainerID), data)
	r.dataCacheLock.Unlock()

	r.removePendingScan(sb.ContainerID)

	seclog.Infof("new sbom generated for '%s': %d files added", sb.ContainerID, data.files.len())

	// Notify policy generator if callback is set
	r.policyGenLock.RLock()
	cb := r.policyGeneratorCb
	r.policyGenLock.RUnlock()

	if cb != nil {
		// Generate policy definition from SBOM packages
		policyDef := r.generateSBOMPolicyDef(sb.ContainerID, report)
		if policyDef != nil {
			cb(string(sb.workloadKey), sb.ContainerID, policyDef)
		}
	}

	return nil
}

func (r *Resolver) getSBOM(containerID containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	sbom := r.hostSBOM
	if containerID != "" {
		sbom, _ = r.sboms.Get(containerID)
	}
	return sbom
}

// ResolvePackage returns the Package that owns the provided file. Make sure the internal fields of "file" are properly
// resolved.
func (r *Resolver) ResolvePackage(containerID containerutils.ContainerID, file *model.FileEvent) *sbomtypes.Package {
	sbom := r.getSBOM(containerID)
	if sbom == nil {
		return nil
	}

	sbom.Lock()
	defer sbom.Unlock()

	pkg := sbom.data.files.queryFile(file.PathnameStr)
	if pkg != nil {
		pkg.LastAccess = time.Now()
	}

	return pkg
}

// newSBOM (thread unsafe) creates a new SBOM entry for the sbom designated by the provided process cache
// entry
func (r *Resolver) newSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	sbom := NewSBOM(id, cgroup, workloadKey)
	r.sboms.Add(id, sbom)
	return sbom
}

// queueWorkload inserts the provided sbom in a SBOM resolver chan, it will be inserted in the scanChan or the
// delayerChan depending on the tags that have been resolved
func (r *Resolver) queueWorkload(sbom *SBOM) {
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.state.Load() != pendingState {
		// this sbom was deleted before we could scan it, ignore it
		return
	}

	// check if this sbom has been scanned before
	r.dataCacheLock.Lock()
	defer r.dataCacheLock.Unlock()

	if data, ok := r.dataCache.Get(workloadKey(sbom.ContainerID)); ok {
		sbom.data = data

		sbom.state.Store(computedState)

		r.sbomsCacheHit.Inc()
		return
	}
	r.sbomsCacheMiss.Inc()

	r.triggerScan(sbom)
}

func (r *Resolver) triggerScan(sbom *SBOM) {
	if !r.addPendingScan(sbom.ContainerID) {
		r.deleteSBOM(sbom)
		return
	}

	// push sbom to the scanner chan
	select {
	case r.scanChan <- sbom:
	default:
		r.removePendingScan(sbom.ContainerID)
		r.deleteSBOM(sbom)
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(workload *tags.Workload) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	if workload == nil {
		return
	}

	id := workload.GCroupCacheEntry.GetContainerID()
	// We don't scan hosts for now
	if len(id) == 0 {
		return
	}

	_, ok := r.sboms.Get(id)
	if !ok {
		workloadKey := getWorkloadKey(workload.Selector.Copy())
		sbom := r.newSBOM(id, workload.GCroupCacheEntry, workloadKey)
		r.queueWorkload(sbom)
	}
}

// GetWorkload returns the sbom of a provided ID
func (r *Resolver) GetWorkload(id containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	if id == "" {
		return r.hostSBOM
	}

	sbom, _ := r.sboms.Get(id)
	return sbom
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(cgroup *cgroupModel.CacheEntry) {
	if !cgroup.IsContainerContextNull() {
		r.Delete(cgroup.GetContainerContext().ContainerID)
	}
}

// Delete removes the SBOM of the provided cgroup id
func (r *Resolver) Delete(id containerutils.ContainerID) {
	sbom := r.GetWorkload(id)
	if sbom == nil {
		return
	}
	sbom.Lock()
	defer sbom.Unlock()

	// Remove this SBOM
	r.deleteSBOM(sbom)
}

// deleteSBOM delete all data indexed by the provided container ID
func (r *Resolver) deleteSBOM(sbom *SBOM) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	seclog.Infof("deleting SBOM entry for '%s'", sbom.ContainerID)

	// should be called under sbom.Lock
	r.sboms.Remove(sbom.ContainerID)
}

// SetPolicyGeneratorCallback sets a callback to be called when an SBOM is computed
// This callback can be used to generate policies based on the SBOM data
func (r *Resolver) SetPolicyGeneratorCallback(cb func(workloadKey string, containerID containerutils.ContainerID, policyDef *rules.PolicyDef)) {
	r.policyGenLock.Lock()
	defer r.policyGenLock.Unlock()
	r.policyGeneratorCb = cb
}

// SendStats sends stats
func (r *Resolver) SendStats() error {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	if val := float64(r.sboms.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	if val := r.sbomGenerations.Swap(0); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMGenerations, int64(val), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMGenerations: %w", err)
		}
	}

	r.dataCacheLock.Lock()
	defer r.dataCacheLock.Unlock()
	if val := float64(r.dataCache.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverSBOMCacheLen, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheLen: %w", err)
		}
	}

	if val := int64(r.sbomsCacheHit.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheHit, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheHit: %w", err)
		}
	}

	if val := int64(r.sbomsCacheMiss.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheMiss, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheMiss: %w", err)
		}
	}

	if val := int64(r.failedSBOMGenerations.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverFailedSBOMGenerations, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverFailedSBOMGenerations: %w", err)
		}
	}

	return nil
}
