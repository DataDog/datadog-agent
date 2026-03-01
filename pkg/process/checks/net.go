// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"net/http"
	"net/netip"
	"runtime"
	"sort"
	"strconv"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/benbjohnson/clock"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	hostinfoutils "github.com/DataDog/datadog-agent/pkg/util/hostinfo"
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
func NewConnectionsCheck(config, sysprobeYamlConfig pkgconfigmodel.Reader, syscfg *sysconfigtypes.Config, wmeta workloadmeta.Component, npCollector npcollector.Component, tagger tagger.Component) *ConnectionsCheck {
	return &ConnectionsCheck{
		config:             config,
		syscfg:             syscfg,
		sysprobeYamlConfig: sysprobeYamlConfig,
		wmeta:              wmeta,
		npCollector:        npCollector,
		tagger:             tagger,
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

	dockerFilter     *parser.DockerProxy
	serviceExtractor *parser.ServiceExtractor
	processData      *ProcessData

	localresolver *resolver.LocalResolver
	wmeta         workloadmeta.Component

	npCollector npcollector.Component

	sysprobeClient *http.Client

	hostTagProvider *hosttags.HostTagProvider
	tagger          tagger.Component
	clock           clock.Clock
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
	c.hostTagProvider = hosttags.NewHostTagProviderWithDuration(c.sysprobeYamlConfig.GetDuration("system_probe_config.expected_tags_duration"))

	// LocalResolver is a singleton LocalResolver
	sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()
	if err != nil {
		return err
	}
	c.clock = clock.New()
	c.localresolver = resolver.NewLocalResolver(sharedContainerProvider, c.clock, maxResolverAddrCacheSize, maxResolverPidCacheSize)
	c.localresolver.Run()

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
	return npmModuleEnabled && c.syscfg.Enabled && !c.sysprobeYamlConfig.GetBool("network_config.direct_send")
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

	conns, err := c.getConnections()
	if err != nil {
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

	c.scheduleNetworkPath(conns)

	getContainersCB := c.getContainerTagsCallback(c.getContainersForExplicitTagging(conns.Conns))
	getProcessTagsCB := c.getProcessTagsCallback()
	iisTags := fetchIISTagsCache(c.sysprobeClient)
	procCacheTags := fetchProcessCacheTags(c.sysprobeClient)
	groupID := nextGroupID()
	messages := batchConnections(c.hostInfo, c.hostTagProvider, getContainersCB, getProcessTagsCB, c.maxConnsPerMessage, groupID, conns.Conns, conns.Dns, c.networkID, conns.ConnTelemetryMap, conns.CompilationTelemetryByAsset, conns.KernelHeaderFetchResult, conns.CORETelemetryByAsset, conns.PrebuiltEBPFAssets, conns.Domains, conns.Routes, conns.Tags, conns.AgentConfiguration, c.serviceExtractor, iisTags, procCacheTags)
	return StandardRunResult(messages), nil
}

// Cleanup frees any resource held by the ConnectionsCheck before the agent exits
func (c *ConnectionsCheck) Cleanup() {
	c.localresolver.Stop()
}

func (c *ConnectionsCheck) scheduleNetworkPath(conns *model.Connections) {
	c.npCollector.ScheduleNetworkPathTests(func(yield func(npmodel.NetworkPathConnection) bool) {
		for _, conn := range conns.Conns {
			srcIP, err := netip.ParseAddr(conn.Laddr.GetIp())
			if err != nil {
				continue
			}
			src := netip.AddrPortFrom(srcIP, uint16(conn.Laddr.Port))
			dstIP, err := netip.ParseAddr(conn.Raddr.GetIp())
			if err != nil {
				continue
			}
			dest := netip.AddrPortFrom(dstIP, uint16(conn.Raddr.Port))
			transDest := dest
			if conn.IpTranslation != nil && conn.IpTranslation.ReplDstIP != "" {
				transDestIP, err := netip.ParseAddr(conn.IpTranslation.ReplDstIP)
				if err == nil {
					transDest = netip.AddrPortFrom(transDestIP, uint16(conn.Raddr.Port))
				}
			}

			npc := npmodel.NetworkPathConnection{
				Source:            src,
				Dest:              dest,
				TranslatedDest:    transDest,
				SourceContainerID: conn.Laddr.GetContainerId(),
				Domain:            getDNSNameForIP(conns, conn.Raddr.GetIp()),
				Type:              conn.Type,
				Direction:         conn.Direction,
				Family:            conn.Family,
				IntraHost:         conn.IntraHost,
				SystemProbeConn:   conn.SystemProbeConn,
			}
			if !yield(npc) {
				return
			}
		}
	})
}

func getDNSNameForIP(conns *model.Connections, ip string) string {
	var domain string
	if dnsEntry := conns.Dns[ip]; dnsEntry != nil && len(dnsEntry.Names) > 0 {
		// We are only using the first entry for now, but in the future, if we find a good solution,
		// we might want to report the other DNS names too if necessary (need more investigation on how to best achieve that).
		domain = dnsEntry.Names[0]
	}
	return domain
}

// getContainersForExplicitTagging returns all containers that are relevant for explicit tagging based on the current connections.
// A container is relevant for explicit tagging if it appears as a local container in the given connections, and
// it started less than `expected_tags_duration` ago, or the agent start time is within the `expected_tags_duration` window.
func (c *ConnectionsCheck) getContainersForExplicitTagging(currentConnections []*model.Connection) map[string]types.EntityID {
	// Get a list of all container IDs that are currently belong with the given connections.
	ids := make(map[string]struct{})
	for _, conn := range currentConnections {
		if conn.Laddr.ContainerId != "" {
			ids[conn.Laddr.ContainerId] = struct{}{}
		}
	}

	currentTime := c.clock.Now()
	duration := c.sysprobeYamlConfig.GetDuration("system_probe_config.expected_tags_duration")
	withinAgentStartingPeriod := pkgconfigsetup.StartTime.Add(duration).After(currentTime)

	res := make(map[string]types.EntityID, len(ids))
	// Iterate through the workloadmeta containers, and for the containers whose IDs are in the `ids` map (a.k.a, relevant
	// containers), check if the container started less than `duration` ago. If so, we consider it relevant for explicit
	// tagging and map the container ID to its EntityID.
	_ = c.wmeta.ListContainersWithFilter(func(container *workloadmeta.Container) bool {
		_, ok := ids[container.ID]
		if !ok {
			return false
		}

		// Either the container started less than `duration` ago, or the agent start time is within the `duration` window.
		if withinAgentStartingPeriod || container.State.StartedAt.Add(duration).After(currentTime) {
			res[container.ID] = types.NewEntityID(types.ContainerID, container.ID)
		}
		// No need to actually return the container instance, as we already extracted the relevant information.
		return false
	})
	return res
}

// getContainerTagsCallback returns a callback that returns the container tags for the given `id`, if the
// container is relevant for explicit tagging.
func (c *ConnectionsCheck) getContainerTagsCallback(relevantContainers map[string]types.EntityID) func(string) ([]string, error) {
	return func(id string) ([]string, error) {
		if entityID, ok := relevantContainers[id]; ok {
			return c.tagger.Tag(entityID, types.HighCardinality)
		}
		// If the container is not relevant for explicit tagging, we return an empty slice.
		return nil, nil
	}
}

// getProcessTagsCallback returns a callback that returns the process tags for the given pid.
func (c *ConnectionsCheck) getProcessTagsCallback() func(int32) ([]string, error) {
	return func(pid int32) ([]string, error) {
		if pid <= 0 {
			return nil, nil
		}
		processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(pid)))
		return c.tagger.Tag(processEntityID, types.HighCardinality)
	}
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
	hostTagProvider *hosttags.HostTagProvider,
	containerTagProvider func(string) ([]string, error),
	processTagProvider func(int32) ([]string, error),
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
	iisTags map[string][]string,
	procCacheTags map[uint32][]string,
) []model.MessageBody {
	groupSize := groupSize(len(cxs), maxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	dnsEncoder := model.NewV2DNSEncoder()

	// Build listening port -> PID map for fallback remote service resolution
	// when IIS tags are not available for same-host connections.
	portToPID := getListeningPortToPIDMap()
	if portToPID == nil {
		portToPID = make(map[int32]int32)
		for _, c := range cxs {
			if c.Pid > 0 {
				portToPID[c.Laddr.Port] = c.Pid
			}
		}
	}

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

		connectionsTagsEncoder := model.NewV3TagEncoder()
		tagsEncoder := model.NewV3TagEncoder()
		// Adding a dummy tag to ensure the indices we get are always >= 0.
		_ = tagsEncoder.Encode([]string{"-"})

		for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
			if entries, ok := dns[c.Raddr.Ip]; ok {
				if _, present := batchDNS[c.Raddr.Ip]; !present {
					// first, walks through and converts entries of type DNSEntry to DNSDatabaseEntry,
					// so that we're always sending the same (newer) type.
					convertDNSEntry(batchDNS, namemap, &namedb, c.Raddr.Ip, entries)
				}
			}

			c.LocalContainerTagsIndex = -1
			if c.Laddr.ContainerId != "" {
				ctrIDForPID[c.Pid] = c.Laddr.ContainerId
				if containerTagProvider != nil {
					if entityTags, err := containerTagProvider(c.Laddr.ContainerId); err != nil {
						log.Debugf("error getting tags for container %s: %v", c.Laddr.ContainerId, err)
					} else if len(entityTags) > 0 {
						c.LocalContainerTagsIndex = int32(tagsEncoder.Encode(entityTags))
					}
				}
			}

			// remap functions create a new map; the map is by string _index_ (not offset)
			// in the namedb.  Each unique string should only occur once.
			remapDNSStatsByDomain(c, namemap, &namedb, domains)
			remapDNSStatsByDomainByQueryType(c, namemap, &namedb, domains)

			// tags remap
			serviceCtx := serviceExtractor.GetServiceContext(c.Pid)
			tagsStr := convertAndEnrichWithServiceCtx(tags, c.Tags, serviceCtx...)

			// For same-host connections, resolve and attach the remote service tags.
			// Try IIS ETW cache first; fall back to PID-based process_context resolution.
			c.RemoteServiceTagsIdx = -1
			if c.IntraHost {
				var remoteTags []string

				// Try IIS tags from system-probe ETW cache
				if iisTags != nil {
					iisKey := fmt.Sprintf("%d-%d", c.Raddr.Port, c.Laddr.Port)
					if iisCachedTags, ok := iisTags[iisKey]; ok {
						remoteTags = append(remoteTags, iisCachedTags...)
					}
				}

				// Fallback: resolve by destination PID using process_context, tagger, and process cache tags
				if len(remoteTags) == 0 {
					if destPID, ok := portToPID[c.Raddr.Port]; ok && destPID != c.Pid {
						destServiceCtx := serviceExtractor.GetServiceContext(destPID)
						remoteTags = append(remoteTags, destServiceCtx...)

						// tagger process tags (service, env, version, tracer metadata)
						if processTagProvider != nil {
							if destProcessTags, err := processTagProvider(destPID); err != nil {
								log.Debugf("error getting process tags for remote pid %d: %v", destPID, err)
							} else {
								remoteTags = append(remoteTags, destProcessTags...)
							}
						}

						// process cache tags from system-probe (env vars: DD_SERVICE, DD_ENV, DD_VERSION, etc.)
						if procCacheTags != nil {
							if cacheTags, ok := procCacheTags[uint32(destPID)]; ok {
								remoteTags = append(remoteTags, cacheTags...)
							}
						}
					}
				}

				if len(remoteTags) > 0 {
					c.RemoteServiceTagsIdx = int32(tagsEncoder.Encode(remoteTags))
					log.Debugf("remote service tags: pid=%d -> raddr.port=%d remoteServiceTagsIdx=%d tags=%v",
						c.Pid, c.Raddr.Port, c.RemoteServiceTagsIdx, remoteTags)
				}
			}

			// Get process tags and add them to the connection tags
			if processTagProvider != nil {
				if processTags, err := processTagProvider(c.Pid); err != nil {
					log.Debugf("error getting tags for process %v: %v", c.Pid, err)
				} else {
					tagsStr = append(tagsStr, processTags...)
				}
			}

			if len(tagsStr) > 0 {
				c.Tags = nil
				c.TagsIdx = int32(connectionsTagsEncoder.Encode(tagsStr))
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

		// remap resolv.conf indices for this batch
		resolvConfSet := indexedset.New[string](0)
		for _, c := range batchConns {
			if c.ResolvConfIdx >= 0 && int(c.ResolvConfIdx) < len(resolvConfs) {
				c.ResolvConfIdx = resolvConfSet.Add(resolvConfs[c.ResolvConfIdx])
			} else {
				c.ResolvConfIdx = -1
			}
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

		hostTagsIndex := -1
		// Add host tags if needed
		if hostTags := hostTagProvider.GetHostTags(); len(hostTags) > 0 {
			hostTagsIndex = tagsEncoder.Encode(hostTags)
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
			EncodedConnectionsTags: connectionsTagsEncoder.Buffer(),
			EncodedTags:            tagsEncoder.Buffer(),
			HostTagsIndex:          int32(hostTagsIndex),
			ResolvConfs:            resolvConfSet.UniqueKeys(),
		}

		// Add OS telemetry
		if hostInfo := hostinfoutils.GetInformation(); hostInfo != nil {
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
	if tagCount == 0 {
		return nil
	}
	tagsStr := make([]string, 0, tagCount)
	for _, t := range tagOffsets {
		tagsStr = append(tagsStr, tags[t])
	}
	tagsStr = append(tagsStr, serviceCtxs...)
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
