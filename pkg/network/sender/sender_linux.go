// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package sender handles sending CNM data to the backend
package sender

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"iter"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/zstd"
	"go4.org/intern"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const clientID = "local_client"
const telemetrySubsystem = "sender__connections"

var senderTelemetry = struct {
	queueSize       telemetry.Gauge
	queueBytes      telemetry.Gauge
	connectionCount telemetry.Counter
}{
	telemetry.NewGauge(telemetrySubsystem, "queue_size", nil, ""),
	telemetry.NewGauge(telemetrySubsystem, "queue_bytes", nil, ""),
	telemetry.NewCounter(telemetrySubsystem, "connection_count", nil, ""),
}

// New creates a direct sender
func New(
	ctx context.Context,
	tr ConnectionsSource,
	deps Dependencies,
) (Sender, error) {
	if err := tr.RegisterClient(clientID); err != nil {
		return nil, fmt.Errorf("register client: %s", err)
	}

	hostName, err := deps.Hostname.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("hostname: %s", err)
	}

	rootPID, err := kernel.RootNSPID()
	if err != nil {
		return nil, fmt.Errorf("root ns PID: %s", err)
	}

	networkID, err := retryGetNetworkID(ctx)
	if err != nil {
		deps.Logger.Infof("network ID not detected: %s", err)
	}

	agentVersion, _ := version.Agent()
	staticHeaders := make(http.Header)
	staticHeaders.Set(headers.HostHeader, hostName)
	staticHeaders.Set(headers.ProcessVersionHeader, agentVersion.GetNumber())
	staticHeaders.Set(headers.ContainerCountHeader, "0")
	staticHeaders.Set(headers.ContentTypeHeader, headers.ProtobufContentType)
	staticHeaders.Set(headers.AgentStartTime, strconv.FormatInt(time.Now().Unix(), 10))
	staticHeaders.Set(headers.PayloadSource, flavor.GetFlavor())
	staticHeaders.Set(headers.ProcessesEnabled, "false")
	staticHeaders.Set(headers.ServiceDiscoveryEnabled, "false")

	queueSize := deps.Config.GetInt("process_config.queue_size")
	if queueSize <= 0 {
		deps.Logger.Warnf("Invalid check queue size: %d. Using default value: %d", queueSize, pkgconfigsetup.DefaultProcessQueueSize)
		queueSize = pkgconfigsetup.DefaultProcessQueueSize
	}
	queueBytes := deps.Config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		deps.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, pkgconfigsetup.DefaultProcessQueueBytes)
		queueBytes = pkgconfigsetup.DefaultProcessQueueBytes
	}
	processAPIEndpoints, err := endpoint.GetAPIEndpoints(deps.Config)
	if err != nil {
		return nil, err
	}
	resolvers, err := resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(processAPIEndpoints))
	if err != nil {
		return nil, err
	}
	forwarderOpts := defaultforwarder.NewOptionsWithResolvers(deps.Config, deps.Logger, resolvers)
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes

	checkInterval := 30 * time.Second
	if deps.Config.IsConfigured("process_config.intervals.connections") {
		if v := deps.Config.GetInt("process_config.intervals.connections"); v > 0 {
			checkInterval = time.Duration(v) * time.Second
		}
	}

	hash := fnv.New32()
	_, _ = hash.Write([]byte(hostName))
	_, _ = hash.Write([]byte(strconv.Itoa(rootPID)))
	hostNamePIDHash := (uint64(hash.Sum32()) & hashMask) << chunkNumberOfBits

	syscfg := deps.Sysprobeconfig
	ctx, cancel := context.WithCancel(ctx)
	ds := directSender{
		tracer: tr,

		hostTagProvider: hosttags.NewHostTagProviderWithDuration(syscfg.GetDuration("system_probe_config.expected_tags_duration")),
		agentCfg: &model.AgentConfiguration{
			NpmEnabled: syscfg.GetBool("network_config.enabled"),
			UsmEnabled: syscfg.GetBool("service_monitoring_config.enabled"),
			CcmEnabled: syscfg.GetBool("ccm_network_config.enabled"),
			CsmEnabled: syscfg.GetBool("runtime_security_config.enabled"),
		},
		ctx:        ctx,
		cancelFunc: cancel,
		resolver:   newContainerResolver(deps.Wmeta, syscfg.GetDuration("system_probe_config.expected_tags_duration")),

		sysprobeconfig: syscfg,
		tagger:         deps.Tagger,
		wmeta:          deps.Wmeta,
		log:            deps.Logger,
		forwarder:      deps.Forwarder,
		npCollector:    deps.NPCollector,

		networkID:           networkID,
		hostname:            hostName,
		containerHostType:   getContainerHostType(),
		sysProbePID:         uint32(rootPID),
		requestIDCachedHash: hostNamePIDHash,

		maxConnsPerMessage: syscfg.SysProbeObject().MaxConnsPerMessage,
		queryTypeEnabled:   syscfg.GetBool("network_config.enable_dns_by_querytype"),
		dnsDomainsEnabled:  syscfg.GetBool("system_probe_config.collect_dns_domains"),

		staticHeaders: staticHeaders,
		resultsQueue:  api.NewWeightedQueue(queueSize, int64(queueBytes)),
		checkInterval: checkInterval,
	}
	if err := ds.encodeHeader(); err != nil {
		return nil, err
	}

	ds.start()
	return &ds, nil
}

type directSender struct {
	tracer  ConnectionsSource
	groupID atomic.Int32

	hostTagProvider *hosttags.HostTagProvider
	agentCfg        *model.AgentConfiguration

	sysprobeconfig sysprobeconfig.Component
	tagger         tagger.Component
	wmeta          workloadmeta.Component
	log            log.Component
	forwarder      connectionsforwarder.Component
	npCollector    npcollector.Component

	networkID         string
	hostname          string
	containerHostType model.ContainerHostType
	sysProbePID       uint32

	maxConnsPerMessage int
	queryTypeEnabled   bool
	dnsDomainsEnabled  bool

	ctx        context.Context
	cancelFunc context.CancelFunc
	resolver   *containerResolver

	// Used to cache the hash result of the host name and the pid of the system-probe.
	// Being used as part of getRequestID method.
	requestIDCachedHash uint64
	staticHeaders       http.Header
	staticEncodedHeader []byte
	resultsQueue        *api.WeightedQueue
	checkInterval       time.Duration
	runCount            uint64
}

func (d *directSender) start() {
	d.log.Info("direct sender started")
	d.resolver.start(d.ctx)
	go d.submitLoop()
	go func() {
		ticker := time.NewTicker(d.checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-d.ctx.Done():
				d.resultsQueue.Stop()
				return
			case <-ticker.C:
				d.collect()
			}
		}
	}()
}

// Stop stops the direct sender
func (d *directSender) Stop() {
	d.cancelFunc()
	d.log.Info("direct sender stopped")
}

func (d *directSender) submitLoop() {
	for {
		item, ok := d.resultsQueue.Poll()
		if !ok {
			return
		}
		allBatches := item.(result)
		for _, p := range allBatches.payloads {
			forwarderPayload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p.body})
			responses, err := d.forwarder.SubmitConnectionChecks(forwarderPayload, p.headers)
			if err != nil {
				d.log.Errorf("Unable to submit payload: %s", err)
				continue
			}
			d.readResponseStatuses(responses)
		}
		senderTelemetry.queueSize.Set(float64(d.resultsQueue.Len()))
		senderTelemetry.queueBytes.Set(float64(d.resultsQueue.Weight()))
	}
}

var networkProtocolToModel = map[network.ConnectionType]model.ConnectionType{
	network.TCP: model.ConnectionType_tcp,
	network.UDP: model.ConnectionType_udp,
}

func (d *directSender) networkPathConnections(conns *network.Connections) iter.Seq[npmodel.NetworkPathConnection] {
	return func(yield func(npmodel.NetworkPathConnection) bool) {
		for _, conn := range conns.Conns {
			src := netip.AddrPortFrom(conn.Source.Addr, conn.SPort)
			dest := netip.AddrPortFrom(conn.Dest.Addr, conn.DPort)
			transDest := dest
			if conn.IPTranslation != nil && conn.IPTranslation.ReplDstIP.IsValid() {
				transDest = netip.AddrPortFrom(conn.IPTranslation.ReplDstIP.Addr, conn.DPort)
			}

			srcContainerID := ""
			if conn.ContainerID.Source != nil {
				srcContainerID, _ = conn.ContainerID.Source.Get().(string)
			}

			npc := npmodel.NetworkPathConnection{
				Source:            src,
				Dest:              dest,
				TranslatedDest:    transDest,
				SourceContainerID: srcContainerID,
				Type:              networkProtocolToModel[conn.Type],
				Direction:         formatDirection(conn.Direction),
				Family:            formatFamily(conn.Family),
				Domain:            getDNSNameForIP(conns, conn.Dest),
				IntraHost:         conn.IntraHost,
				SystemProbeConn:   conn.Pid == d.sysProbePID,
			}
			if !yield(npc) {
				return
			}
		}
	}
}

func (d *directSender) collect() {
	start := time.Now()
	d.runCount++

	conns, cleanup, err := d.tracer.GetActiveConnections(clientID)
	if err != nil {
		d.log.Errorf("error getting connections: %s", err)
		return
	}
	senderTelemetry.connectionCount.Add(float64(len(conns.Conns)))
	defer cleanup()
	defer network.Reclaim(conns)

	if dsc := directSenderConsumerInstance.Load(); dsc != nil {
		dsc.proxyFilter.FilterProxies(conns)
		defer dsc.cleanupProcesses()
	}

	d.npCollector.ScheduleNetworkPathTests(d.networkPathConnections(conns))

	groupID := d.groupID.Add(1)

	allBatches := result{payloads: make([]payload, 0, d.batchCount(conns))}
	messageIndex := 0
	for body := range d.batches(conns, groupID) {
		extraHeaders := d.staticHeaders.Clone()
		extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(start.Unix())))
		requestID := d.getRequestID(start, messageIndex)
		d.log.Debugf("the request id of the current message: %s", requestID)
		extraHeaders.Set(headers.RequestIDHeader, requestID)

		allBatches.payloads = append(allBatches.payloads, payload{
			body:    body,
			headers: extraHeaders,
		})
		allBatches.size += int64(len(body))
		messageIndex++
	}

	// we have to include all batches as a single item in the queue, otherwise individual batches could be evicted.
	d.resultsQueue.Add(allBatches)
	d.logCheckDuration(start)

	senderTelemetry.queueSize.Set(float64(d.resultsQueue.Len()))
	senderTelemetry.queueBytes.Set(float64(d.resultsQueue.Weight()))
}

func (d *directSender) batches(conns *network.Connections, groupID int32) iter.Seq[[]byte] {
	messageIndex := 0
	numBatches := d.batchCount(conns)
	dnsEncoder := model.NewV2DNSEncoder()
	tagsSet := indexedset.New[string](0)

	usmEncoders := marshal.InitializeUSMEncoders(conns)
	d.resolver.resolveDestinationContainerIDs(conns)

	// Sort connections by remote IP/PID for more efficient resolution
	slices.SortFunc(conns.Conns, func(a, b network.ConnectionStats) int {
		if a.Dest.Addr != b.Dest.Addr {
			return a.Dest.Addr.Compare(b.Dest.Addr)
		}
		return int(a.Pid) - int(b.Pid)
	})

	builder := model.NewCollectorConnectionsBuilder(io.Discard)

	return func(yield func([]byte) bool) {
		defer func() {
			for _, e := range usmEncoders {
				e.Close()
			}
		}()
		defer d.resolver.removeDeadTagContainers()

		for connsChunk := range slices.Chunk(conns.Conns, d.maxConnsPerMessage) {
			// TODO is there some way to get a larger lower bound on the size of a payload
			// ex: (minConnSize * len(connsChunk)) + len(d.staticEncodedHeader)
			// compression makes this calculation difficult
			dstBuf := bytes.NewBuffer(make([]byte, 0, len(d.staticEncodedHeader)))
			_, err := dstBuf.Write(d.staticEncodedHeader)
			if err != nil {
				d.log.Errorf("Unable to encode message header: %s", err)
				continue
			}
			zw := zstd.NewWriter(dstBuf)

			builder.Reset(zw)
			d.encodeConfiguration(builder)
			builder.SetGroupId(groupID)
			builder.SetGroupSize(numBatches)

			if messageIndex == 0 {
				d.encodeTelemetry(conns, builder)
			}

			d.encodeContainerForPID(connsChunk, builder)
			dnsSet, indexToOffset := d.encodeDNS(connsChunk, conns, dnsEncoder, builder)
			resolvConfSet := indexedset.New[network.ResolvConf](0)
			routeSet := indexedset.New[network.Via](0)
			connectionsTagsEncoder := model.NewV3TagEncoder()
			tagsEncoder := model.NewV3TagEncoder()
			// Adding a dummy tag to ensure the indices we get are always >= 0.
			_ = tagsEncoder.Encode([]string{"-"})

			for _, nc := range connsChunk {
				builder.AddConnections(func(builder *model.ConnectionBuilder) {
					d.encodeConnection(builder, nc, conns, routeSet, resolvConfSet)
					d.addContainerTags(builder, nc.ContainerID.Source, tagsEncoder)
					d.addTags(builder, nc, tagsSet, usmEncoders, connectionsTagsEncoder)
					d.addDNS(builder, nc, dnsSet, indexToOffset)
				})
			}

			// this must come before we access tagsEncoder.Buffer()
			builder.SetHostTagsIndex(int32(tagsEncoder.Encode(d.hostTagProvider.GetHostTags())))
			builder.SetEncodedTags(func(b *bytes.Buffer) {
				b.Write(tagsEncoder.Buffer())
			})
			builder.SetEncodedConnectionsTags(func(b *bytes.Buffer) {
				b.Write(connectionsTagsEncoder.Buffer())
			})
			d.encodeRoutes(routeSet, builder)
			for _, v := range resolvConfSet.UniqueKeys() {
				builder.AddResolvConfs(v.Get())
			}

			if err := zw.Close(); err != nil {
				d.log.Errorf("Unable to close compression writer: %s", err)
				continue
			}

			if !yield(dstBuf.Bytes()) {
				return
			}
			messageIndex++
		}
	}
}

func getInternedString(v *intern.Value) string {
	if v == nil {
		return ""
	}
	if s, ok := v.Get().(string); ok {
		return s
	}
	return ""
}

const (
	secondsNumberOfBits = 22
	hashNumberOfBits    = 28
	chunkNumberOfBits   = 14
	secondsMask         = 1<<secondsNumberOfBits - 1
	hashMask            = 1<<hashNumberOfBits - 1
	chunkMask           = 1<<chunkNumberOfBits - 1
)

// getRequestID generates a unique identifier (string representation of 64 bits integer) that is composed as follows:
//  1. 22 bits of the seconds in the current month.
//  2. 28 bits of hash of the hostname and system-probe pid.
//  3. 14 bits of the current message in the batch being sent to the server.
func (d *directSender) getRequestID(start time.Time, chunkIndex int) string {
	// The epoch is the beginning of the month of the `start` variable.
	epoch := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	// We are taking the seconds in the current month, and representing them under 22 bits.
	// In a month we have 60 seconds per minute * 60 minutes per hour * 24 hours per day * maximum 31 days a month
	// which is 2678400, and it can be represented with log2(2678400) = 21.35 bits.
	seconds := (uint64(start.Sub(epoch).Seconds()) & secondsMask) << (hashNumberOfBits + chunkNumberOfBits)

	// Next, we take up to 14 bits to represent the message index in the batch.
	// It means that we support up to 16384 (2 ^ 14) different messages being sent on the same batch.
	chunk := uint64(chunkIndex & chunkMask)
	return strconv.FormatUint(seconds+d.requestIDCachedHash+chunk, 10)
}

func (d *directSender) readResponseStatuses(responses <-chan defaultforwarder.Response) {
	checkName := "connections"

	for response := range responses {
		if response.Err != nil {
			d.log.Errorf("[%s] Error from %s: %s", checkName, response.Domain, response.Err)
			continue
		}

		if response.StatusCode >= 300 {
			d.log.Errorf("[%s] Invalid response from %s: %d -> %v", checkName, response.Domain, response.StatusCode, response.Err)
			continue
		}

		r, err := model.DecodeMessage(response.Body)
		if err != nil {
			d.log.Errorf("[%s] Could not decode response body: %s", checkName, err)
			continue
		}

		switch r.Header.Type {
		case model.TypeResCollector:
			rm := r.Body.(*model.ResCollector)
			if len(rm.Message) > 0 {
				d.log.Errorf("[%s] Error in response from %s: %s", checkName, response.Domain, rm.Message)
			}
		default:
			d.log.Errorf("[%s] Unexpected response type from %s: %d", checkName, response.Domain, r.Header.Type)
		}
	}
}

func (d *directSender) batchCount(conns *network.Connections) int32 {
	numBatches := int32(len(conns.Conns) / d.maxConnsPerMessage)
	if len(conns.Conns)%d.maxConnsPerMessage > 0 {
		numBatches++
	}
	return numBatches
}

func (d *directSender) logCheckDuration(start time.Time) {
	elapsed := time.Since(start)
	switch {
	case d.runCount < 5:
		d.log.Infof("Finished connections check #%d in %s", d.runCount, elapsed)
	case d.runCount == 5:
		d.log.Infof("Finished connections check #%d in %s. First 5 check runs finished, next runs will be logged every 20 runs.", d.runCount, elapsed)
	case d.runCount%20 == 0:
		d.log.Infof("Finish connections check #%d in %s", d.runCount, elapsed)
	}
}
