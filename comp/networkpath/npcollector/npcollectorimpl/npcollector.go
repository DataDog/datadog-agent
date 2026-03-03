// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npcollectorimpl implements network path collector
package npcollectorimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/netip"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/connfilter"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/pathteststore"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/networkfilter"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	reverseDNSLookupMetricPrefix        = common.NetworkPathCollectorMetricPrefix + "reverse_dns_lookup."
	reverseDNSLookupFailuresMetricName  = reverseDNSLookupMetricPrefix + "failures"
	reverseDNSLookupSuccessesMetricName = reverseDNSLookupMetricPrefix + "successes"
	netpathConnsSkippedMetricName       = common.NetworkPathCollectorMetricPrefix + "schedule.conns_skipped"
)

type npCollectorImpl struct {
	// config related
	collectorConfigs *collectorConfigs
	sourceExcludes   []*networkfilter.ConnectionFilter
	destExcludes     []*networkfilter.ConnectionFilter

	// Deps
	epForwarder  eventplatform.Forwarder
	logger       log.Component
	statsdClient ddgostatsd.ClientInterface
	rdnsquerier  rdnsquerier.Component

	// Counters
	receivedPathtestCount    *atomic.Uint64
	processedTracerouteCount *atomic.Uint64

	// Pathtest store
	pathtestStore          *pathteststore.Store
	pathtestInputChan      chan *common.Pathtest
	pathtestProcessingChan chan *pathteststore.PathtestContext

	// Scheduling related
	running               bool
	workers               int
	stopChan              chan struct{}
	flushLoopDone         chan struct{}
	workersDone           chan struct{}
	pathtestsListenerDone chan struct{}
	flushInterval         time.Duration
	inputChanFullLogLimit *utillog.Limit

	traceroute traceroute.Component

	// structures needed to ease mocking/testing
	TimeNowFn func() time.Time

	networkDevicesNamespace string
	filter                  *connfilter.ConnFilter
}

func newNoopNpCollectorImpl() *npCollectorImpl {
	return &npCollectorImpl{
		collectorConfigs: &collectorConfigs{},
	}
}

func newNpCollectorImpl(epForwarder eventplatform.Forwarder, collectorConfigs *collectorConfigs, traceroute traceroute.Component, logger log.Component, rdnsquerier rdnsquerier.Component, statsd ddgostatsd.ClientInterface) *npCollectorImpl {
	logger.Infof("New NpCollector %+v", collectorConfigs)
	filter, errs := connfilter.NewConnFilter(collectorConfigs.filterConfig, collectorConfigs.ddSite, collectorConfigs.monitorIPWithoutDomain)

	if len(errs) > 0 {
		logger.Errorf("connection filter errors: %s", errors.Join(errs...))
	}

	return &npCollectorImpl{
		collectorConfigs: collectorConfigs,
		sourceExcludes:   networkfilter.ParseConnectionFilters(collectorConfigs.sourceExcludedConns),
		destExcludes:     networkfilter.ParseConnectionFilters(collectorConfigs.destExcludedConns),

		epForwarder:  epForwarder,
		logger:       logger,
		statsdClient: statsd,
		rdnsquerier:  rdnsquerier,

		pathtestStore:          pathteststore.NewPathtestStore(collectorConfigs.storeConfig, logger, statsd, time.Now),
		pathtestInputChan:      make(chan *common.Pathtest, collectorConfigs.pathtestInputChanSize),
		pathtestProcessingChan: make(chan *pathteststore.PathtestContext, collectorConfigs.pathtestProcessingChanSize),
		flushInterval:          collectorConfigs.flushInterval,
		workers:                collectorConfigs.workers,
		inputChanFullLogLimit:  utillog.NewLogLimit(10, time.Minute*5),

		networkDevicesNamespace: collectorConfigs.networkDevicesNamespace,

		receivedPathtestCount:    atomic.NewUint64(0),
		processedTracerouteCount: atomic.NewUint64(0),
		TimeNowFn:                time.Now,

		traceroute: traceroute,

		stopChan:              make(chan struct{}),
		pathtestsListenerDone: make(chan struct{}),
		flushLoopDone:         make(chan struct{}),
		workersDone:           make(chan struct{}),

		filter: filter,
	}
}

// makePathtest extracts pathtest information using a single connection and the connection check's reverse dns map
func (s *npCollectorImpl) makePathtest(conn npmodel.NetworkPathConnection) common.Pathtest {
	protocol := modelProtocolToPayload[conn.Type]
	if s.collectorConfigs.icmpMode.ShouldUseICMP(protocol) {
		protocol = payload.ProtocolICMP
	}

	var remotePort uint16
	// only TCP traces can be done to the active port
	if protocol == payload.ProtocolTCP {
		remotePort = conn.Dest.Port()
	}

	hostname := conn.Dest.Addr().String()
	if conn.Domain != "" {
		hostname = conn.Domain
	}

	return common.Pathtest{
		Hostname:          hostname,
		Port:              remotePort,
		Protocol:          protocol,
		SourceContainerID: conn.SourceContainerID,
		Metadata: common.PathtestMetadata{
			ReverseDNSHostname: conn.Domain,
		},
	}
}

func doSubnetsContainIP(subnets []netip.Prefix, ip netip.Addr) bool {
	for _, subnet := range subnets {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *npCollectorImpl) checkPassesConnCIDRFilters(conn npmodel.NetworkPathConnection, vpcSubnets []netip.Prefix) bool {
	if len(vpcSubnets) == 0 && len(s.sourceExcludes) == 0 && len(s.destExcludes) == 0 {
		// this should be most customers - parsing IPs is not necessary
		return true
	}

	if !conn.TranslatedDest.IsValid() {
		conn.TranslatedDest = conn.Dest
	}

	if doSubnetsContainIP(vpcSubnets, conn.TranslatedDest.Addr()) {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_intra_vpc"}, 1)
		return false
	}

	filterable := networkfilter.FilterableConnection{
		Type:   conn.Type,
		Source: conn.Source,
		Dest:   conn.TranslatedDest,
	}
	if networkfilter.IsExcludedConnection(s.sourceExcludes, s.destExcludes, filterable) {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_cidr_excluded"}, 1)
		return false
	}

	return true
}

func (s *npCollectorImpl) shouldScheduleNetworkPathForConn(conn npmodel.NetworkPathConnection, vpcSubnets []netip.Prefix) bool {
	if conn.IntraHost {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_intra_host"}, 1)
		return false
	}
	if conn.SystemProbeConn {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_system_probe_conn"}, 1)
		return false
	}
	if conn.Direction != model.ConnectionDirection_outgoing {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_incoming"}, 1)
		return false
	}
	// only ipv4 is supported currently
	// if domain is present, we will traceroute the domain, so, it doesn't matter if the conn family is IPv4 or IPv6
	if conn.Domain == "" && conn.Family != model.ConnectionFamily_v4 {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_ipv6"}, 1)
		return false
	}

	if !s.checkPassesConnCIDRFilters(conn, vpcSubnets) {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_not_matched_by_conn_filters"}, 1)
		return false
	}

	if !s.filter.IsIncluded(conn.Domain, conn.Dest.Addr()) {
		_ = s.statsdClient.Incr(netpathConnsSkippedMetricName, []string{"reason:skip_not_matched_by_filters"}, 1)
		return false
	}

	return true
}

func (s *npCollectorImpl) getVPCSubnets() ([]netip.Prefix, error) {
	if !s.collectorConfigs.disableIntraVPCCollection {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	vpcSubnets, err := network.GetVPCSubnetsForHost(ctx)
	if err != nil {
		return nil, fmt.Errorf("disable_intra_vpc_collection is enforced, but failed to get VPC subnets: %w", err)
	}

	return vpcSubnets, nil
}

func (s *npCollectorImpl) ScheduleNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection]) {
	if !s.collectorConfigs.connectionsMonitoringEnabled {
		return
	}
	vpcSubnets, err := s.getVPCSubnets()
	if err != nil {
		s.logger.Errorf("Failed to get VPC subnets to skip: %s", err)
		return
	}

	startTime := s.TimeNowFn()
	connCount := 0
	for conn := range conns {
		connCount++
		if !s.shouldScheduleNetworkPathForConn(conn, vpcSubnets) {
			s.logger.Tracef("Skipped connection: addr=%s, protocol=%s", conn.Dest, conn.Type)
			continue
		}
		pathtest := s.makePathtest(conn)
		err := s.scheduleOne(&pathtest)
		if err != nil {
			s.logger.Errorf("Error scheduling pathtests: %s", err)
		}
	}
	_ = s.statsdClient.Count(common.NetworkPathCollectorMetricPrefix+"schedule.conns_received", int64(connCount), []string{}, 1)
	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"schedule.duration", s.TimeNowFn().Sub(startTime).Seconds(), nil, 1)
}

// scheduleOne schedules pathtests.
// It shouldn't block, if the input channel is full, an error is returned.
func (s *npCollectorImpl) scheduleOne(pathtest *common.Pathtest) error {
	if s.pathtestInputChan == nil {
		return errors.New("no input channel, please check that network path is enabled")
	}
	s.logger.Debugf("Schedule traceroute for: hostname=%s port=%d", pathtest.Hostname, pathtest.Port)

	_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"schedule.pathtest_count", []string{}, 1)
	select {
	case s.pathtestInputChan <- pathtest:
		_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"schedule.pathtest_processed", []string{}, 1)
		return nil
	default:
		_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"schedule.pathtest_dropped", []string{"reason:input_chan_full"}, 1)
		if s.inputChanFullLogLimit.ShouldLog() {
			s.logger.Warnf("collector input channel is full (channel capacity is %d)", cap(s.pathtestInputChan))
		}
		return nil
	}
}

func (s *npCollectorImpl) start() error {
	if s.running {
		return errors.New("server already started")
	}
	s.running = true

	s.logger.Info("Start NpCollector")

	go s.listenPathtests()
	go s.flushLoop()
	go s.runWorkers()

	return nil
}

func (s *npCollectorImpl) stop() {
	s.logger.Info("Stop NpCollector")
	if !s.running {
		return
	}
	close(s.stopChan)
	<-s.flushLoopDone
	<-s.workersDone
	<-s.pathtestsListenerDone
	s.running = false
}

func (s *npCollectorImpl) listenPathtests() {
	s.logger.Debug("Starting listening for pathtests")
	for {
		select {
		case <-s.stopChan:
			s.logger.Info("Stopped listening for pathtests")
			s.pathtestsListenerDone <- struct{}{}
			return
		case ptest := <-s.pathtestInputChan:
			s.logger.Debugf("Pathtest received: %+v", ptest)
			s.receivedPathtestCount.Inc()
			s.pathtestStore.Add(ptest)
		}
	}
}

func (s *npCollectorImpl) runTracerouteForPath(ptest *pathteststore.PathtestContext) {
	s.logger.Debugf("Run Traceroute for ptest: %+v", ptest)

	cfg := config.Config{
		DestHostname:              ptest.Pathtest.Hostname,
		DestPort:                  ptest.Pathtest.Port,
		MaxTTL:                    uint8(s.collectorConfigs.maxTTL),
		Timeout:                   s.collectorConfigs.timeout,
		Protocol:                  ptest.Pathtest.Protocol,
		TCPMethod:                 s.collectorConfigs.tcpMethod,
		TCPSynParisTracerouteMode: s.collectorConfigs.tcpSynParisTracerouteMode,
		DisableWindowsDriver:      s.collectorConfigs.disableWindowsDriver,
		ReverseDNS:                false, // Do not run reverse DNS in datadog-traceroute, it's handled in npcollector
		TracerouteQueries:         s.collectorConfigs.tracerouteQueries,
		E2eQueries:                s.collectorConfigs.e2eQueries,
	}

	s.logger.Debugf("Running traceroute with config: %+v", cfg)

	path, err := s.traceroute.Run(context.TODO(), cfg)
	if err != nil {
		s.logger.Errorf("run traceroute error: %s", err)
		return
	}

	err = payload.ValidateNetworkPath(&path)
	if err != nil {
		s.logger.Errorf("failed to validate network path: %s", err)
		return
	}

	path.Source.ContainerID = ptest.Pathtest.SourceContainerID
	path.Namespace = s.networkDevicesNamespace
	path.Origin = payload.PathOriginNetworkTraffic
	path.TestRunType = payload.TestRunTypeDynamic
	path.SourceProduct = s.collectorConfigs.sourceProduct
	path.CollectorType = payload.CollectorTypeAgent

	// Perform reverse DNS lookup on destination and hop IPs
	s.enrichPathWithRDNS(&path, ptest.Pathtest.Metadata.ReverseDNSHostname)

	payloadBytes, err := json.Marshal(path)
	if err != nil {
		s.logger.Errorf("json marshall error: %s", err)
	} else {
		s.logger.Debugf("network path event: %s", string(payloadBytes))
		m := message.NewMessage(payloadBytes, nil, "", 0)
		err = s.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkPath)
		if err != nil {
			s.logger.Errorf("failed to send event to epForwarder: %s", err)
		}
	}
}

func (s *npCollectorImpl) flushLoop() {
	s.logger.Debugf("Starting flush loop")

	flushTicker := time.NewTicker(s.flushInterval)

	var lastFlushTime time.Time
	for {
		select {
		// stop sequence
		case <-s.stopChan:
			s.logger.Info("Stopped flush loop")
			s.flushLoopDone <- struct{}{}
			flushTicker.Stop()
			return
		// automatic flush sequence
		case flushTime := <-flushTicker.C:
			s.flushWrapper(flushTime, lastFlushTime)
			lastFlushTime = flushTime
		}
	}
}

func (s *npCollectorImpl) flushWrapper(flushTime time.Time, lastFlushTime time.Time) {
	s.logger.Debugf("Flush loop at %s", flushTime)
	if !lastFlushTime.IsZero() {
		flushInterval := flushTime.Sub(lastFlushTime)
		_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"flush.interval", flushInterval.Seconds(), []string{}, 1)
	}

	s.flush()
	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"flush.duration", s.TimeNowFn().Sub(flushTime).Seconds(), []string{}, 1)
}

func (s *npCollectorImpl) flush() {
	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"workers", float64(s.workers), []string{}, 1)

	flushTime := s.TimeNowFn()
	pathtestsToFlush := s.pathtestStore.Flush()

	flowsContexts := s.pathtestStore.GetContextsCount()
	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"pathtest_store_size", float64(flowsContexts), []string{}, 1)
	s.logger.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(pathtestsToFlush), time.Since(flushTime).Milliseconds(), flowsContexts)

	_ = s.statsdClient.Count(common.NetworkPathCollectorMetricPrefix+"flush.pathtest_count", int64(len(pathtestsToFlush)), []string{}, 1)
	for _, ptConf := range pathtestsToFlush {
		s.logger.Tracef("flushed ptConf %s:%d", ptConf.Pathtest.Hostname, ptConf.Pathtest.Port)
		select {
		case s.pathtestProcessingChan <- ptConf:
			_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"flush.pathtest_processed", []string{}, 1)
		default:
			_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"flush.pathtest_dropped", []string{"reason:processing_chan_full"}, 1)
			s.logger.Tracef("collector processing channel is full (channel capacity is %d)", cap(s.pathtestProcessingChan))
		}
	}

	// keep this metric after the flows are flushed
	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"processing_chan_size", float64(len(s.pathtestProcessingChan)), []string{}, 1)

	_ = s.statsdClient.Gauge(common.NetworkPathCollectorMetricPrefix+"input_chan_size", float64(len(s.pathtestInputChan)), []string{}, 1)
}

// enrichPathWithRDNS populates a NetworkPath with reverse-DNS queried hostnames.
func (s *npCollectorImpl) enrichPathWithRDNS(path *payload.NetworkPath, knownDestHostname string) {
	if !s.collectorConfigs.reverseDNSEnabled {
		return
	}

	// collect unique IP addresses from destination and hops
	ipSet := make(map[string]struct{})

	for _, run := range path.Traceroute.Runs {
		// only look up the destination hostname if we need to
		if knownDestHostname == "" {
			ipSet[run.Destination.IPAddress.String()] = struct{}{}
		}
		for _, hop := range run.Hops {
			if !hop.Reachable {
				continue
			}
			ipSet[hop.IPAddress.String()] = struct{}{}
		}
	}
	ipAddrs := make([]string, 0, len(ipSet))
	for ip := range ipSet {
		ipAddrs = append(ipAddrs, ip)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.collectorConfigs.reverseDNSTimeout)
	defer cancel()

	// perform reverse DNS lookup on destination and hops
	results := s.rdnsquerier.GetHostnames(ctx, ipAddrs)
	if len(results) != len(ipAddrs) {
		_ = s.statsdClient.Incr(reverseDNSLookupMetricPrefix+"results_length_mismatch", []string{}, 1)
		s.logger.Debugf("Reverse lookup failed for all hops in path from %s to %s", path.Source.Hostname, path.Destination.Hostname)
	}

	for i := range path.Traceroute.Runs {
		run := &path.Traceroute.Runs[i]
		// assign resolved hostnames to destination and hops
		if knownDestHostname != "" {
			run.Destination.ReverseDNS = []string{knownDestHostname}
		} else {
			// TODO: We should make reverse DNS return all available DNS names, not just one.
			hostname := s.getReverseDNSResult(run.Destination.IPAddress.String(), results)
			// if hostname is blank, use what's given by traceroute
			// TODO: would it be better to move the logic up from the traceroute command?
			// benefit to the current approach is having consistent behavior for all paths
			// both static and dynamic
			if hostname != "" {
				run.Destination.ReverseDNS = []string{hostname}
			}
		}

		for j := range run.Hops {
			hop := &run.Hops[j]
			if !hop.Reachable {
				continue
			}
			ipAddr := hop.IPAddress.String()
			rDNS := s.getReverseDNSResult(ipAddr, results)
			if rDNS != "" {
				hop.ReverseDNS = []string{rDNS}
			}
		}
	}
}

func (s *npCollectorImpl) getReverseDNSResult(ipAddr string, results map[string]rdnsquerier.ReverseDNSResult) string {
	result, ok := results[ipAddr]
	if !ok {
		_ = s.statsdClient.Incr(reverseDNSLookupFailuresMetricName, []string{"reason:absent"}, 1)
		s.logger.Tracef("Reverse DNS lookup failed for IP %s", ipAddr)
		return ""
	}
	if result.Err != nil {
		_ = s.statsdClient.Incr(reverseDNSLookupFailuresMetricName, []string{"reason:error"}, 1)
		s.logger.Tracef("Reverse lookup failed for hop IP %s: %s", ipAddr, result.Err)
		return ""
	}
	if result.Hostname == "" {
		_ = s.statsdClient.Incr(reverseDNSLookupSuccessesMetricName, []string{"status:empty"}, 1)
	} else {
		_ = s.statsdClient.Incr(reverseDNSLookupSuccessesMetricName, []string{"status:found"}, 1)
	}
	return result.Hostname
}

func (s *npCollectorImpl) runWorkers() {
	s.logger.Debugf("Starting workers (%d)", s.workers)

	var wg sync.WaitGroup
	for w := 0; w < s.workers; w++ {
		wg.Add(1)
		s.logger.Debugf("Starting worker #%d", w)
		go func() {
			defer wg.Done()
			s.runWorker(w)
		}()
	}
	wg.Wait()
	s.workersDone <- struct{}{}
}

func (s *npCollectorImpl) runWorker(workerID int) {
	for {
		select {
		case <-s.stopChan:
			s.logger.Debugf("[worker%d] Stopped worker", workerID)
			return
		case pathtestCtx := <-s.pathtestProcessingChan:
			s.logger.Debugf("[worker%d] Handling pathtest hostname=%s, port=%d", workerID, pathtestCtx.Pathtest.Hostname, pathtestCtx.Pathtest.Port)
			startTime := s.TimeNowFn()

			s.runTracerouteForPath(pathtestCtx)
			s.processedTracerouteCount.Inc()

			checkInterval := pathtestCtx.LastFlushInterval()
			checkDuration := s.TimeNowFn().Sub(startTime)
			_ = s.statsdClient.Histogram(common.NetworkPathCollectorMetricPrefix+"worker.task_duration", checkDuration.Seconds(), nil, 1)
			_ = s.statsdClient.Incr(common.NetworkPathCollectorMetricPrefix+"worker.pathtest_processed", []string{}, 1)
			if checkInterval > 0 {
				_ = s.statsdClient.Histogram(common.NetworkPathCollectorMetricPrefix+"worker.pathtest_interval", checkInterval.Seconds(), nil, 1)
			}
		}
	}
}
