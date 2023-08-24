// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"fmt"
	"io"
	nnet "net"
	"sort"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
	putil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/proto/connectionserver"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/subscriptions"
)

var (
	// LocalResolver is a singleton LocalResolver
	LocalResolver = &resolver.LocalResolver{}

	// ErrTracerStillNotInitialized signals that the tracer is _still_ not ready, so we shouldn't log additional errors
	ErrTracerStillNotInitialized = errors.New("remote tracer is still not initialized")

	// ProcessAgentClientID process-agent unique ID
	ProcessAgentClientID = "process-agent-unique-id"
)

// NewConnectionsCheck returns an instance of the ConnectionsCheck.
func NewConnectionsCheck(config, sysprobeYamlConfig config.ConfigReader, syscfg *sysconfig.Config) *ConnectionsCheck {
	return &ConnectionsCheck{
		config:             config,
		syscfg:             syscfg,
		sysprobeYamlConfig: sysprobeYamlConfig,
	}
}

// ConnectionsCheck collects statistics about live TCP and UDP connections.
type ConnectionsCheck struct {
	syscfg             *sysconfig.Config
	sysprobeYamlConfig config.ConfigReader
	config             config.ConfigReader

	hostInfo               *HostInfo
	maxConnsPerMessage     int
	tracerClientID         string
	networkID              string
	notInitializedLogLimit *putil.LogLimit

	dockerFilter     *parser.DockerProxy
	serviceExtractor *parser.ServiceExtractor
	processData      *ProcessData

	processConnRatesTransmitter subscriptions.Transmitter[ProcessConnRates]
}

// ProcessConnRates describes connection rates for processes
type ProcessConnRates map[int32]*model.ProcessNetworks

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(syscfg *SysProbeConfig, hostInfo *HostInfo) error {
	c.hostInfo = hostInfo
	c.maxConnsPerMessage = syscfg.MaxConnsPerMessage
	c.notInitializedLogLimit = putil.NewLogLimit(1, time.Minute*10)

	// We use the current process PID as the system-probe client ID
	c.tracerClientID = ProcessAgentClientID

	// Calling the remote tracer will cause it to initialize and check connectivity
	tu, err := net.GetRemoteSystemProbeUtil(syscfg.SystemProbeAddress)

	if err != nil {
		log.Warnf("could not initiate connection with system probe: %s", err)
	} else {
		// Register process agent as a system probe's client
		// This ensures we start recording data from now to the first call to `Run`
		err = tu.Register(c.tracerClientID)
		if err != nil {
			log.Warnf("could not register process-agent to system-probe: %s", err)
		}
	}

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID
	c.processData = NewProcessData(c.config)
	c.dockerFilter = parser.NewDockerProxy()
	c.serviceExtractor = parser.NewServiceExtractor(c.sysprobeYamlConfig)
	c.processData.Register(c.dockerFilter)
	c.processData.Register(c.serviceExtractor)

	return nil
}

// IsEnabled returns true if the check is enabled by configuration
func (c *ConnectionsCheck) IsEnabled() bool {
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

func (c *ConnectionsCheck) runGRPC(nextGroupID int32) (RunResult, error) {
	// Create a context with a timeout of 10 seconds
	timedContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(_ context.Context, _ string) (nnet.Conn, error) {
			return nnet.Dial(net.NetType, c.syscfg.GRPCSocketFilePath)
		}),
		grpc.WithInsecure(),
	}

	// the second parameter is unused because we use a custom  dialer that has the state to see the address.
	conn, err := grpc.DialContext(timedContext, "unused", opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to dial with context to grpc server due to: %v", err)
	}
	defer conn.Close()
	client := connectionserver.NewSystemProbeClient(conn)

	response, err := client.GetConnections(timedContext, &connectionserver.GetConnectionsRequest{ClientID: c.tracerClientID}, grpc.MaxCallRecvMsgSize(api.MaxGRPCServerMessage))
	if err != nil {
		return nil, fmt.Errorf("unable to get connections from grpc server due to: %v", err)
	}

	// We only need to fetch once for a batch, and this boolean indicates that.
	processFetchSucceeded := true
	if err := c.processData.Fetch(); err != nil {
		processFetchSucceeded = false
		log.Warnf("error collecting processes for filter and extraction: %s", err)
	}

	unmarshaler := netEncoding.GetUnmarshaler(netEncoding.ContentTypeProtobuf)
	var batchMessages []model.MessageBody

	for {
		start := time.Now()
		res, err := response.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to get respinse from grpc server due to: %v", err)
		}

		batch, err := unmarshaler.Unmarshal(res.Data)
		if err != nil {
			return nil, fmt.Errorf("unable to unmarshal response due to: %v", err)
		}
		log.Debugf("[grpc] the number of connections in grpc is %d", len(batch.Conns))
		if !processFetchSucceeded {
			c.dockerFilter.Filter(batch)
		}

		// Resolve the Raddr side of connections for local containers
		LocalResolver.Resolve(batch)

		c.notifyProcessConnRates(c.config, batch)

		log.Debugf("collected connections in %s", time.Since(start))

		// When calling the processConnectionsBatch function, the last parameter is used to indicate whether we are
		// in the first batch. This indicator is utilized to include telemetry specifically in the initial batch.
		// In the process agent, the expectation is to  incorporate telemetry for individual connections solely
		// within the first batch.
		batchMessages = append(batchMessages, processConnectionsBatch(c.hostInfo, c.maxConnsPerMessage, nextGroupID, batch.Conns, batch.Dns, c.networkID,
			batch.ConnTelemetryMap, batch.CompilationTelemetryByAsset, batch.KernelHeaderFetchResult,
			batch.CORETelemetryByAsset, batch.PrebuiltEBPFAssets, batch.Domains, batch.Routes, batch.Tags,
			batch.AgentConfiguration, c.serviceExtractor, len(batchMessages) == 0))
		netEncoding.ConnsToPool(batch)
	}
	return StandardRunResult(batchMessages), nil
}

func (c *ConnectionsCheck) runHTTP(nextGroupID int32) (RunResult, error) {
	start := time.Now()

	conns, err := c.getConnections()
	if err != nil {
		// If the tracer is not initialized, or still not initialized, then we want to exit without error'ing
		if err == ebpf.ErrNotImplemented || err == ErrTracerStillNotInitialized {
			return nil, nil
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
	LocalResolver.Resolve(conns)

	c.notifyProcessConnRates(c.config, conns)

	log.Debugf("collected connections in %s", time.Since(start))

	groupID := nextGroupID
	messages := batchConnections(c.hostInfo, c.maxConnsPerMessage, groupID, conns.Conns, conns.Dns, c.networkID, conns.ConnTelemetryMap, conns.CompilationTelemetryByAsset, conns.KernelHeaderFetchResult, conns.CORETelemetryByAsset, conns.PrebuiltEBPFAssets, conns.Domains, conns.Routes, conns.Tags, conns.AgentConfiguration, c.serviceExtractor)
	return StandardRunResult(messages), nil
}

// Run runs the ConnectionsCheck to collect the active network connections
// and any closed network connections since the last Run.
// For each connection we'll return a `model.Connection`
// that will be bundled up into a `CollectorConnections`.
// See agent.proto for the schema of the message and models.
func (c *ConnectionsCheck) Run(nextGroupID func() int32, _ *RunOptions) (RunResult, error) {
	if c.syscfg.GRPCServerEnabled {
		return c.runGRPC(nextGroupID())
	}
	return c.runHTTP(nextGroupID())
}

// Cleanup frees any resource held by the ConnectionsCheck before the agent exits
func (c *ConnectionsCheck) Cleanup() {}

func (c *ConnectionsCheck) getConnections() (*model.Connections, error) {
	tu, err := net.GetRemoteSystemProbeUtil(c.syscfg.SocketAddress)
	if err != nil {
		if c.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection: %v (will only log every 10 minutes)", err)
		}
		return nil, ErrTracerStillNotInitialized
	}
	return tu.GetConnections(c.tracerClientID)
}

func (c *ConnectionsCheck) notifyProcessConnRates(config config.ConfigReader, conns *model.Connections) {
	if len(c.processConnRatesTransmitter.Chs) == 0 {
		return
	}

	connCheckIntervalS := int(GetInterval(config, ConnectionsCheckName) / time.Second)

	connRates := make(ProcessConnRates)
	for _, c := range conns.Conns {
		rates, ok := connRates[c.Pid]
		if !ok {
			connRates[c.Pid] = &model.ProcessNetworks{ConnectionRate: 1, BytesRate: float32(c.LastBytesReceived) + float32(c.LastBytesSent)}
			continue
		}

		rates.BytesRate += float32(c.LastBytesSent) + float32(c.LastBytesReceived)
		rates.ConnectionRate++
	}

	for _, rates := range connRates {
		rates.BytesRate /= float32(connCheckIntervalS)
		rates.ConnectionRate /= float32(connCheckIntervalS)
	}

	c.processConnRatesTransmitter.Notify(connRates)
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
	if old == nil || len(old) == 0 {
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

// handleConnections collects information from a batch of connections.
func handleConnections(
	cxs []*model.Connection,
	dns map[string]*model.DNSEntry,
	batchDNS map[string]*model.DNSDatabaseEntry,
	namemap map[string]int32,
	namedb []string,
	ctrIDForPID map[int32]string,
	domains []string,
	serviceExtractor *parser.ServiceExtractor,
	tags []string,
	tagsEncoder model.TagEncoder,
	routes []*model.Route,
	dnsEncoder model.DNSEncoderV2,
	agentCfg *model.AgentConfiguration,
	hostInfo *HostInfo,
	networkID string,
	groupID int32,
	groupSize int32) *model.CollectorConnections {
	for _, c := range cxs { // We only want to include DNS entries relevant to this batch of connections
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
	for _, c := range cxs {
		if c.RouteIdx < 0 {
			continue
		}
		if i, ok := newRouteIndices[c.RouteIdx]; ok {
			c.RouteIdx = i
			continue
		}

		new := int32(len(newRouteIndices))
		newRouteIndices[c.RouteIdx] = new
		batchRoutes = append(batchRoutes, routes[c.RouteIdx])
		c.RouteIdx = new
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
		for _, c := range cxs { // We only want to include DNS entries relevant to this batch of connections
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
		for _, c := range cxs { // We only want to include DNS entries relevant to this batch of connections
			remapDNSStatsByOffset(c, indexToOffset)
		}
	}
	cc := &model.CollectorConnections{
		AgentConfiguration:     agentCfg,
		HostName:               hostInfo.HostName,
		NetworkId:              networkID,
		Connections:            cxs,
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

	return cc
}

// updateConnectionsTelemetry collects information about the current state of connections and updates the telemetry
// metrics accordingly.
func updateConnectionsTelemetry(
	cc *model.CollectorConnections,
	connTelemetryMap map[string]int64,
	compilationTelemetry map[string]*model.RuntimeCompilationTelemetry,
	kernelHeaderFetchResult model.KernelHeaderFetchResult,
	coreTelemetry map[string]model.COREResult,
	prebuiltAssets []string) {
	cc.ConnTelemetryMap = connTelemetryMap
	cc.CompilationTelemetryByAsset = compilationTelemetry
	cc.KernelHeaderFetchResult = kernelHeaderFetchResult
	cc.CORETelemetryByAsset = coreTelemetry
	cc.PrebuiltEBPFAssets = prebuiltAssets
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
	sortConnections(cxs, maxConnsPerMessage)

	for len(cxs) > 0 {
		batchSize := min(maxConnsPerMessage, len(cxs))
		batchConns := cxs[:batchSize] // Connections for this particular batch

		ctrIDForPID := make(map[int32]string)
		batchDNS := make(map[string]*model.DNSDatabaseEntry)
		namemap := make(map[string]int32)
		namedb := make([]string, 0)

		tagsEncoder := model.NewV2TagEncoder()

		cc := handleConnections(batchConns,
			dns,
			batchDNS,
			namemap,
			namedb,
			ctrIDForPID,
			domains,
			serviceExtractor,
			tags,
			tagsEncoder,
			routes,
			dnsEncoder,
			agentCfg,
			hostInfo,
			networkID,
			groupID,
			groupSize)

		// only add the telemetry to the first message to prevent double counting
		if len(batches) == 0 {
			updateConnectionsTelemetry(
				cc,
				connTelemetryMap,
				compilationTelemetry,
				kernelHeaderFetchResult,
				coreTelemetry,
				prebuiltAssets)
		}
		batches = append(batches, cc)

		cxs = cxs[batchSize:]
	}
	return batches
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// sortConnections sort connections by remote IP/PID for more efficient resolution
func sortConnections(cxs []*model.Connection, maxConnsPerMessage int) {
	if len(cxs) > maxConnsPerMessage {
		sort.Slice(cxs, func(i, j int) bool {
			if cxs[i].Raddr.Ip != cxs[j].Raddr.Ip {
				return cxs[i].Raddr.Ip < cxs[j].Raddr.Ip
			}
			return cxs[i].Pid < cxs[j].Pid
		})
	}
}

// Manage connections that have been split into batches from the system-probe
func processConnectionsBatch(
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
	isFirst bool,
) model.MessageBody {
	groupSize := groupSize(len(cxs), maxConnsPerMessage)

	dnsEncoder := model.NewV2DNSEncoder()
	sortConnections(cxs, maxConnsPerMessage)

	ctrIDForPID := make(map[int32]string)
	batchDNS := make(map[string]*model.DNSDatabaseEntry)
	namemap := make(map[string]int32)
	namedb := make([]string, 0)

	tagsEncoder := model.NewV2TagEncoder()

	cc := handleConnections(cxs,
		dns,
		batchDNS,
		namemap,
		namedb,
		ctrIDForPID,
		domains,
		serviceExtractor,
		tags,
		tagsEncoder,
		routes,
		dnsEncoder,
		agentCfg,
		hostInfo,
		networkID,
		groupID,
		groupSize)

	// only add the telemetry to the first message to prevent double counting
	if isFirst {
		updateConnectionsTelemetry(
			cc,
			connTelemetryMap,
			compilationTelemetry,
			kernelHeaderFetchResult,
			coreTelemetry,
			prebuiltAssets)
	}
	return cc
}
