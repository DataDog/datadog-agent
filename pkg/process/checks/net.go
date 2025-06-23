// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/benbjohnson/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxResolverPidCacheSize  = 32768
	maxResolverAddrCacheSize = 4096
)

var (
	// ProcessAgentClientID process-agent unique ID
	ProcessAgentClientID = "process-agent-unique-id"
)

// NewConnectionsCheck returns an instance of the ConnectionsCheck.
func NewConnectionsCheck(config, sysprobeYamlConfig pkgconfigmodel.Reader, syscfg *sysconfigtypes.Config, wmeta workloadmeta.Component, npCollector npcollector.Component, statsd statsd.ClientInterface) *ConnectionsCheck {
	return &ConnectionsCheck{
		config:             config,
		syscfg:             syscfg,
		sysprobeYamlConfig: sysprobeYamlConfig,
		wmeta:              wmeta,
		npCollector:        npCollector,
		statsd:             statsd,
	}
}

// ConnectionsCheck collects statistics about live TCP and UDP connections.
type ConnectionsCheck struct {
	syscfg             *sysconfigtypes.Config
	sysprobeYamlConfig pkgconfigmodel.Reader
	config             pkgconfigmodel.Reader

	hostInfo               *HostInfo
	maxConnsPerMessage     int
	networkID              string
	notInitializedLogLimit *log.Limit
	lastFullRunTime        time.Time
	guaranteedRunInterval  time.Duration // Use the standard check interval for guaranteed runs

	dockerFilter     *parser.DockerProxy
	serviceExtractor *parser.ServiceExtractor
	processData      *ProcessData

	localresolver *resolver.LocalResolver
	wmeta         workloadmeta.Component

	npCollector npcollector.Component

	sysprobeClient *http.Client
	statsd         statsd.ClientInterface
}

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(syscfg *SysProbeConfig, hostInfo *HostInfo, _ bool) error {
	c.hostInfo = hostInfo
	c.maxConnsPerMessage = syscfg.MaxConnsPerMessage
	c.notInitializedLogLimit = log.NewLogLimit(1, time.Minute*10)
	c.sysprobeClient = sysprobeclient.Get(syscfg.SystemProbeAddress)

	// Register process agent as a system probe's client
	// This ensures we start recording data from now to the first call to `Run`
	err := c.register()
	if err != nil {
		log.Warnf("could not register process-agent to system-probe: %s", err)
	}

	networkID, err := retryGetNetworkID(c.sysprobeClient)
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID
	c.processData = NewProcessData(c.config)
	c.dockerFilter = parser.NewDockerProxy()
	serviceExtractorEnabled := c.sysprobeYamlConfig.GetBool("system_probe_config.process_service_inference.enabled")
	useWindowsServiceName := c.sysprobeYamlConfig.GetBool("system_probe_config.process_service_inference.use_windows_service_name")
	useImprovedAlgorithm := c.sysprobeYamlConfig.GetBool("system_probe_config.process_service_inference.use_improved_algorithm")
	c.serviceExtractor = parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	c.processData.Register(c.dockerFilter)
	c.processData.Register(c.serviceExtractor)

	// LocalResolver is a singleton LocalResolver
	sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()
	if err != nil {
		return err
	}
	c.localresolver = resolver.NewLocalResolver(sharedContainerProvider, clock.New(), maxResolverAddrCacheSize, maxResolverPidCacheSize)
	c.localresolver.Run()

	// Initialize state for the capacity-based run logic only when dynamic interval is enabled
	useDynamicInterval := c.config.GetBool("process_config.connections.enable_dynamic_interval")
	if useDynamicInterval {
		c.lastFullRunTime = time.Time{} // Ensure the first run is a full run
		// Guaranteed run interval is driven by the standard connections check interval
		c.guaranteedRunInterval = GetInterval(c.config, ConnectionsCheckName)
		log.Infof(
			"connections check dynamic interval enabled: Capacity check interval=%v, Guaranteed full run interval=%v",
			ConnectionsCheckDynamicInterval,
			c.guaranteedRunInterval,
		)
	} else {
		log.Infof("connections check running with traditional fixed interval: %v", GetInterval(c.config, ConnectionsCheckName))
	}

	return nil
}

// IsEnabled returns true if the check is enabled by configuration
func (c *ConnectionsCheck) IsEnabled() bool {
	// connection check is not supported on darwin, so we should fail gracefully in this case.
	if runtime.GOOS == "darwin" {
		return false
	}

	// connections check is only supported on the process agent
	if flavor.GetFlavor() != flavor.ProcessAgent {
		return false
	}

	_, npmModuleEnabled := c.syscfg.EnabledModules[sysconfig.NetworkTracerModule]
	return npmModuleEnabled && c.syscfg.Enabled
}

// SupportsRunOptions returns true if the check supports RunOptions
func (c *ConnectionsCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ConnectionsCheck.
func (c *ConnectionsCheck) Name() string { return ConnectionsCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (c *ConnectionsCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *ConnectionsCheck) ShouldSaveLastRun() bool { return false }

// Run runs the ConnectionsCheck to collect the active network connections
// and any closed network connections since the last Run.
// For each connection we'll return a `model.Connection`
// that will be bundled up into a `CollectorConnections`.
// See agent.proto for the schema of the message and models.
func (c *ConnectionsCheck) Run(nextGroupID func() int32, _ *RunOptions) (RunResult, error) {
	start := time.Now()

	// Determine if we should run the check based on configuration and capacity
	if !c.shouldRunCheck(start) {
		return StandardRunResult(nil), nil
	}

	conns, err := c.getConnections()
	if err != nil {
		log.Errorf("failed to get connections: %v", err)
		if c.statsd != nil {
			_ = c.statsd.Count("datadog.process.connections.collection_errors", 1, []string{}, 1)
		}
		return nil, err
	}

	// Filter out (in-place) connection data associated with docker-proxy
	err = c.processData.Fetch()
	if err != nil {
		log.Warnf("error collecting processes for filter and extraction: %s", err)
	} else {
		c.dockerFilter.Filter(conns)
	}
	// Resolve the Raddr side of connections for local containers
	c.localresolver.Resolve(conns)

	log.Debugf("collected connections in %s", time.Since(start))

	if c.statsd != nil {
		_ = c.statsd.Gauge("datadog.process.connections.count", float64(len(conns.Conns)), []string{}, 1)
		_ = c.statsd.Histogram("datadog.process.connections.collection_time", time.Since(start).Seconds(), []string{}, 1)
	}

	c.npCollector.ScheduleConns(conns.Conns, conns.Dns)

	groupID := nextGroupID()
	messages := batchConnections(c.hostInfo, c.maxConnsPerMessage, groupID, conns.Conns, conns.Dns, c.networkID, conns.ConnTelemetryMap, conns.CompilationTelemetryByAsset, conns.KernelHeaderFetchResult, conns.CORETelemetryByAsset, conns.PrebuiltEBPFAssets, conns.Domains, conns.Routes, conns.Tags, conns.AgentConfiguration, c.serviceExtractor)
	return StandardRunResult(messages), nil
}

// shouldRunCheck determines whether the connections check should run based on configuration and system capacity.
// It handles both traditional (always run) and dynamic interval (capacity-aware) modes.
// Returns true if the check should run, false if it should be skipped.
func (c *ConnectionsCheck) shouldRunCheck(start time.Time) bool {
	// Check if dynamic interval is enabled
	useDynamicInterval := c.config.GetBool("process_config.connections.enable_dynamic_interval")

	if !useDynamicInterval {
		// Traditional behavior: always run the check
		status.UpdateLastCollectTime(start)
		log.Debugf("running connections check (dynamic interval disabled, always run)")
		return true
	}

	// Dynamic interval mode: use capacity-aware logic
	timeSinceLastRun := start.Sub(c.lastFullRunTime)
	// Add a small tolerance (250ms) to account for differences in the check interval and the guaranteed run interval
	guaranteedIntervalWithTolerance := c.guaranteedRunInterval - (time.Millisecond * 250)
	isTimeForGuaranteedRun := c.lastFullRunTime.IsZero() || timeSinceLastRun >= guaranteedIntervalWithTolerance

	isNearCapacity := false
	if c.sysprobeClient != nil {
		var capacityErr error
		isNearCapacity, capacityErr = c.checkCapacity()
		if capacityErr != nil {
			log.Warnf("failed to check system-probe connection capacity: %v. Proceeding based on time interval.", capacityErr)
			isNearCapacity = false
			if c.statsd != nil {
				_ = c.statsd.Count("datadog.process.connections.capacity_check_errors", 1, []string{}, 1)
			}
		}
	} else {
		log.Debug("system probe client not available, skipping capacity check.")
	}

	// Decide whether to run the full check
	shouldRunFullCheck := isTimeForGuaranteedRun || isNearCapacity

	if c.statsd != nil {
		if shouldRunFullCheck {
			_ = c.statsd.Count("datadog.process.connections.runs", 1, []string{fmt.Sprintf("guaranteed_run:%t", isTimeForGuaranteedRun), fmt.Sprintf("near_capacity:%t", isNearCapacity)}, 1)
		} else {
			_ = c.statsd.Count("datadog.process.connections.skipped_runs", 1, []string{}, 1)
		}
	}

	if !shouldRunFullCheck {
		log.Debugf("skipping connections check run (Capacity OK, not time for guaranteed run). Last full run: %v ago", timeSinceLastRun)
		return false
	}

	// We're going to run the check, so update state and log
	status.UpdateLastCollectTime(start)
	log.Debugf("running connections check. Reason: TimeForGuaranteedRun=%v (last run %v ago), NearCapacity=%v", isTimeForGuaranteedRun, timeSinceLastRun, isNearCapacity)
	c.lastFullRunTime = start

	return true
}

// Cleanup frees any resource held by the ConnectionsCheck before the agent exits
func (c *ConnectionsCheck) Cleanup() {
	c.localresolver.Stop()
}

func (c *ConnectionsCheck) register() error {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/register?client_id="+ProcessAgentClientID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.sysprobeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("conn request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}
	return nil
}

func (c *ConnectionsCheck) getConnections() (*model.Connections, error) {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/connections?client_id="+ProcessAgentClientID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/protobuf")
	resp, err := c.sysprobeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	return netEncoding.GetUnmarshaler(contentType).Unmarshal(body)
}

func convertDNSEntry(dnstable map[string]*model.DNSDatabaseEntry, namemap map[string]int32, namedb *[]string, ip string, entry *model.DNSEntry) {
	dbentry := &model.DNSDatabaseEntry{
		NameOffsets: make([]int32, 0, len(entry.Names)),
	}
	for _, name := range entry.Names {
		// at this point, the NameOffsets slice is actually a slice of indices into
		// the name slice.  It will be converted prior to encoding.
		if idx, ok := namemap[name]; ok {
			dbentry.NameOffsets = append(dbentry.NameOffsets, idx)
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, name)
			namemap[name] = dblen
			dbentry.NameOffsets = append(dbentry.NameOffsets, dblen)
		}

	}
	dnstable[ip] = dbentry
}

func remapDNSStatsByDomain(c *model.Connection, namemap map[string]int32, namedb *[]string, dnslist []string) {
	old := c.DnsStatsByDomain
	if len(old) == 0 {
		return
	}
	c.DnsStatsByDomain = make(map[int32]*model.DNSStats)
	for key, val := range old {
		// key is the index into the old array (dnslist)
		domainstr := dnslist[key]
		if idx, ok := namemap[domainstr]; ok {
			c.DnsStatsByDomain[idx] = val
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, domainstr)
			namemap[domainstr] = dblen
			c.DnsStatsByDomain[dblen] = val
		}
	}
}

func remapDNSStatsByDomainByQueryType(c *model.Connection, namemap map[string]int32, namedb *[]string, dnslist []string) {
	old := c.DnsStatsByDomainByQueryType
	c.DnsStatsByDomainByQueryType = make(map[int32]*model.DNSStatsByQueryType)
	for key, val := range old {
		// key is the index into the old array (dnslist)
		domainstr := dnslist[key]
		if idx, ok := namemap[domainstr]; ok {
			c.DnsStatsByDomainByQueryType[idx] = val
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, domainstr)
			namemap[domainstr] = dblen
			c.DnsStatsByDomainByQueryType[dblen] = val
		}
	}

}

func remapDNSStatsByOffset(c *model.Connection, indexToOffset []int32) {
	oldByDomain := c.DnsStatsByDomain
	oldByDomainByQueryType := c.DnsStatsByDomainByQueryType

	c.DnsStatsByDomainOffsetByQueryType = make(map[int32]*model.DNSStatsByQueryType)

	// first, walk the stats by domain.  Put them in by query type 'A`
	for key, val := range oldByDomain {
		off := indexToOffset[key]
		if _, ok := c.DnsStatsByDomainOffsetByQueryType[off]; !ok {
			c.DnsStatsByDomainOffsetByQueryType[off] = &model.DNSStatsByQueryType{}
			c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType = make(map[int32]*model.DNSStats)
		}
		c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType[int32(dns.TypeA)] = val
	}
	for key, val := range oldByDomainByQueryType {
		off := indexToOffset[key]
		c.DnsStatsByDomainOffsetByQueryType[off] = val
	}
	c.DnsStatsByDomain = nil
	c.DnsStatsByDomainByQueryType = nil
}

// Connections are split up into a chunks of a configured size conns per message to limit the message size on intake.
func batchConnections(
	hostInfo *HostInfo,
	maxConnsPerMessage int,
	groupID int32,
	cxs []*model.Connection,
	dns map[string]*model.DNSEntry,
	networkID string,
	connTelemetryMap map[string]int64,
	compilationTelemetry map[string]*model.RuntimeCompilationTelemetry,
	kernelHeaderFetchResult model.KernelHeaderFetchResult,
	coreTelemetry map[string]model.COREResult,
	prebuiltAssets []string,
	domains []string,
	routes []*model.Route,
	tags []string,
	agentCfg *model.AgentConfiguration,
	serviceExtractor *parser.ServiceExtractor,
) []model.MessageBody {
	groupSize := groupSize(len(cxs), maxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	dnsEncoder := model.NewV2DNSEncoder()

	if len(cxs) > maxConnsPerMessage {
		// Sort connections by remote IP/PID for more efficient resolution
		sort.Slice(cxs, func(i, j int) bool {
			if cxs[i].Raddr.Ip != cxs[j].Raddr.Ip {
				return cxs[i].Raddr.Ip < cxs[j].Raddr.Ip
			}
			return cxs[i].Pid < cxs[j].Pid
		})
	}

	for len(cxs) > 0 {
		batchSize := min(maxConnsPerMessage, len(cxs))
		batchConns := cxs[:batchSize] // Connections for this particular batch

		ctrIDForPID := make(map[int32]string)
		batchDNS := make(map[string]*model.DNSDatabaseEntry)
		namemap := make(map[string]int32)
		namedb := make([]string, 0)

		tagsEncoder := model.NewV2TagEncoder()

		for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
			if entries, ok := dns[c.Raddr.Ip]; ok {
				if _, present := batchDNS[c.Raddr.Ip]; !present {
					// first, walks through and converts entries of type DNSEntry to DNSDatabaseEntry,
					// so that we're always sending the same (newer) type.
					convertDNSEntry(batchDNS, namemap, &namedb, c.Raddr.Ip, entries)
				}
			}

			if c.Laddr.ContainerId != "" {
				ctrIDForPID[c.Pid] = c.Laddr.ContainerId
			}

			// remap functions create a new map; the map is by string _index_ (not offset)
			// in the namedb.  Each unique string should only occur once.
			remapDNSStatsByDomain(c, namemap, &namedb, domains)
			remapDNSStatsByDomainByQueryType(c, namemap, &namedb, domains)

			// tags remap
			serviceCtx := serviceExtractor.GetServiceContext(c.Pid)
			tagsStr := convertAndEnrichWithServiceCtx(tags, c.Tags, serviceCtx...)

			if len(tagsStr) > 0 {
				c.Tags = nil
				c.TagsIdx = int32(tagsEncoder.Encode(tagsStr))
			} else {
				c.TagsIdx = -1
			}
		}

		// remap route indices
		// map of old index to new index
		newRouteIndices := make(map[int32]int32)
		var batchRoutes []*model.Route
		for _, c := range batchConns {
			if c.RouteIdx < 0 {
				continue
			}
			if i, ok := newRouteIndices[c.RouteIdx]; ok {
				c.RouteIdx = i
				continue
			}

			newIdx := int32(len(newRouteIndices))
			newRouteIndices[c.RouteIdx] = newIdx
			batchRoutes = append(batchRoutes, routes[c.RouteIdx])
			c.RouteIdx = newIdx
		}

		// EncodeDomainDatabase will take the namedb (a simple slice of strings with each unique
		// domain string) and convert it into a buffer of all of the strings.
		// indexToOffset contains the map from the string index to where it occurs in the encodedNameDb
		var mappedDNSLookups []byte
		encodedNameDb, indexToOffset, err := dnsEncoder.EncodeDomainDatabase(namedb)
		if err != nil {
			encodedNameDb = nil
			// since we were unable to properly encode the indexToOffet map, the
			// rest of the maps will now be unreadable by the back-end.  Just clear them
			for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
				c.DnsStatsByDomain = nil
				c.DnsStatsByDomainByQueryType = nil
				c.DnsStatsByDomainOffsetByQueryType = nil
			}
		} else {

			// Now we have all available information.  EncodeMapped with take the string indices
			// that are used, and encode (using the indexToOffset array) the offset into the buffer
			// this way individual strings can be directly accessed on decode.
			mappedDNSLookups, err = dnsEncoder.EncodeMapped(batchDNS, indexToOffset)
			if err != nil {
				mappedDNSLookups = nil
			}
			for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
				remapDNSStatsByOffset(c, indexToOffset)
			}
		}
		cc := &model.CollectorConnections{
			AgentConfiguration:     agentCfg,
			HostName:               hostInfo.HostName,
			NetworkId:              networkID,
			Connections:            batchConns,
			GroupId:                groupID,
			GroupSize:              groupSize,
			ContainerForPid:        ctrIDForPID,
			EncodedDomainDatabase:  encodedNameDb,
			EncodedDnsLookups:      mappedDNSLookups,
			ContainerHostType:      hostInfo.ContainerHostType,
			Routes:                 batchRoutes,
			EncodedConnectionsTags: tagsEncoder.Buffer(),
		}

		// Add OS telemetry
		if hostInfo := hostMetadataUtils.GetInformation(); hostInfo != nil {
			cc.KernelVersion = hostInfo.KernelVersion
			cc.Architecture = hostInfo.KernelArch
			cc.Platform = hostInfo.Platform
			cc.PlatformVersion = hostInfo.PlatformVersion
		}

		// only add the telemetry to the first message to prevent double counting
		if len(batches) == 0 {
			cc.ConnTelemetryMap = connTelemetryMap
			cc.CompilationTelemetryByAsset = compilationTelemetry
			cc.KernelHeaderFetchResult = kernelHeaderFetchResult
			cc.CORETelemetryByAsset = coreTelemetry
			cc.PrebuiltEBPFAssets = prebuiltAssets
		}
		batches = append(batches, cc)

		cxs = cxs[batchSize:]
	}
	return batches
}

func groupSize(total, maxBatchSize int) int32 {
	groupSize := total / maxBatchSize
	if total%maxBatchSize > 0 {
		groupSize++
	}
	return int32(groupSize)
}

// converts the tags based on the tagOffsets for encoding. It also enriches it with service context if any
func convertAndEnrichWithServiceCtx(tags []string, tagOffsets []uint32, serviceCtxs ...string) []string {
	tagCount := len(tagOffsets) + len(serviceCtxs)
	tagsStr := make([]string, 0, tagCount)
	for _, t := range tagOffsets {
		tagsStr = append(tagsStr, tags[t])
	}

	for _, serviceCtx := range serviceCtxs {
		if serviceCtx != "" {
			tagsStr = append(tagsStr, serviceCtx)
		}
	}

	return tagsStr
}

// retryGetNetworkID attempts to fetch the network_id maxRetries times before failing
// as the endpoint is sometimes unavailable during host startup
func retryGetNetworkID(sysProbeClient *http.Client) (string, error) {
	const maxRetries = 4
	var err error
	var networkID string
	for attempt := 1; attempt <= maxRetries; attempt++ {
		networkID, err = getNetworkID(sysProbeClient)
		if err == nil {
			return networkID, nil
		}
		log.Debugf(
			"failed to fetch network ID (attempt %d/%d): %s",
			attempt,
			maxRetries,
			err,
		)
		if attempt < maxRetries {
			time.Sleep(time.Duration(250*attempt) * time.Millisecond)
		}
	}
	return "", fmt.Errorf("failed to get network ID after %d attempts: %w", maxRetries, err)
}

// getNetworkID fetches network_id from the current netNS or from the system probe if necessary, where the root netNS is used
func getNetworkID(sysProbeClient *http.Client) (string, error) {
	networkID, err := network.GetNetworkID(context.Background())
	if err != nil {
		if sysProbeClient == nil {
			return "", fmt.Errorf("no network ID detected and system-probe client not available: %w", err)
		}
		log.Debugf("no network ID detected. retrying via system-probe: %s", err)
		networkID, err = net.GetNetworkID(sysProbeClient)
		if err != nil {
			log.Debugf("failed to get network ID from system-probe: %s", err)
			return "", fmt.Errorf("failed to get network ID from system-probe: %w", err)
		}
	}
	return networkID, err
}

func (c *ConnectionsCheck) checkCapacity() (bool, error) {
	if c.sysprobeClient == nil {
		return false, fmt.Errorf("system probe client is nil")
	}

	capacityCheckStart := time.Now()
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/connections/check_capacity?client_id="+ProcessAgentClientID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("error creating capacity check request %s: %w", url, err)
	}

	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("Checking connections capacity endpoint: %s", url)
	}
	resp, err := c.sysprobeClient.Do(req)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return false, fmt.Errorf("error calling capacity check endpoint %s: %w", url, err)
	}
	defer resp.Body.Close()

	if c.statsd != nil {
		_ = c.statsd.Histogram("datadog.process.connections.capacity_check_time", time.Since(capacityCheckStart).Seconds(), []string{}, 1)
	}

	switch resp.StatusCode {
	case http.StatusOK: // 200 OK => Near capacity
		log.Debugf("Capacity check returned 200 OK (Near Capacity) for client %s", ProcessAgentClientID)
		if c.statsd != nil {
			_ = c.statsd.Count("datadog.process.connections.capacity_near_limit", 1, []string{}, 1)
		}
		return true, nil
	case http.StatusNoContent: // 204 No Content => Not near capacity
		log.Tracef("Capacity check returned 204 No Content (OK) for client %s", ProcessAgentClientID)
		if c.statsd != nil {
			_ = c.statsd.Count("datadog.process.connections.capacity_ok", 1, []string{}, 1)
		}
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code %d from capacity check endpoint %s", resp.StatusCode, url)
	}
}
