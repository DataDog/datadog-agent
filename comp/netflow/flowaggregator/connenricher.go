// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package flowaggregator

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	netEncoding "github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

const netflowEnricherClientID = "netflow-enricher"

// connKey is a lookup key for the connection index.
// IPs are stored as 16-byte arrays (IPv4 addresses are v4-mapped-v6).
type connKey struct {
	SrcIP    [16]byte
	DstIP    [16]byte
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

// connMeta holds enrichment metadata for a matched connection.
type connMeta struct {
	SrcContainerID string
	DstContainerID string
	SrcService     string
	DstService     string
	SrcTags        []string
	DstTags        []string
	Direction      string
}

// ConnEnricher periodically polls system-probe's /connections endpoint and
// builds an in-memory lookup index for enriching NetFlow records with eBPF
// connection metadata (container IDs, service names, tags).
type ConnEnricher struct {
	mu         sync.RWMutex
	exactIndex map[connKey]*connMeta // exact 5-tuple match
	noSrcPort  map[connKey]*connMeta // wildcard SrcPort=0 for ephemeral rollup
	noDstPort  map[connKey]*connMeta // wildcard DstPort=0 for ephemeral rollup

	httpClient   *http.Client
	tagger       tagger.Component
	pollInterval time.Duration
	logger       log.Component
	stopChan     chan struct{}
	doneChan     chan struct{}
	registered   bool
}

// NewConnEnricher creates a new ConnEnricher that polls system-probe for connection data.
func NewConnEnricher(sysprobeSocketPath string, pollInterval time.Duration, logger log.Component, taggerComp tagger.Component) *ConnEnricher {
	return &ConnEnricher{
		exactIndex:   make(map[connKey]*connMeta),
		noSrcPort:    make(map[connKey]*connMeta),
		noDstPort:    make(map[connKey]*connMeta),
		httpClient:   sysprobeclient.Get(sysprobeSocketPath),
		tagger:       taggerComp,
		pollInterval: pollInterval,
		logger:       logger,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start begins background polling of system-probe connections.
func (e *ConnEnricher) Start() {
	e.logger.Info("ConnEnricher starting")
	go e.pollLoop()
}

// Stop signals the poll loop to stop and waits for it to finish.
func (e *ConnEnricher) Stop() {
	e.logger.Info("ConnEnricher stopping")
	close(e.stopChan)
	<-e.doneChan
}

func (e *ConnEnricher) pollLoop() {
	defer close(e.doneChan)

	// Do an initial poll immediately
	e.doPoll()

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.doPoll()
		}
	}
}

func (e *ConnEnricher) doPoll() {
	if !e.registered {
		if err := e.register(); err != nil {
			e.logger.Debugf("ConnEnricher: failed to register with system-probe: %v", err)
			return
		}
		e.registered = true
	}

	conns, err := e.getConnections()
	if err != nil {
		e.logger.Debugf("ConnEnricher: failed to get connections from system-probe: %v", err)
		return
	}

	e.buildIndex(conns)
	e.logger.Debugf("ConnEnricher: indexed %d connections (%d exact, %d noSrcPort, %d noDstPort)",
		len(conns.Conns), len(e.exactIndex), len(e.noSrcPort), len(e.noDstPort))
}

func (e *ConnEnricher) register() error {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/register?client_id="+netflowEnricherClientID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register request failed: status code: %d", resp.StatusCode)
	}
	return nil
}

func (e *ConnEnricher) getConnections() (*model.Connections, error) {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/connections?client_id="+netflowEnricherClientID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/protobuf")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("connections request failed: status code: %d", resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	return netEncoding.GetUnmarshaler(contentType).Unmarshal(body)
}

func (e *ConnEnricher) buildIndex(conns *model.Connections) {
	exact := make(map[connKey]*connMeta, len(conns.Conns))
	noSrc := make(map[connKey]*connMeta, len(conns.Conns))
	noDst := make(map[connKey]*connMeta, len(conns.Conns))

	for _, conn := range conns.Conns {
		if conn.Laddr == nil || conn.Raddr == nil {
			continue
		}

		srcIP := normalizeIP(conn.Laddr.Ip)
		dstIP := normalizeIP(conn.Raddr.Ip)
		proto := connectionTypeToProtocol(conn.Type)

		meta := &connMeta{
			SrcContainerID: conn.Laddr.GetContainerId(),
			DstContainerID: conn.Raddr.GetContainerId(),
			Direction:      directionString(conn.Direction),
		}

		// Resolve tags for source container
		if meta.SrcContainerID != "" {
			meta.SrcService, meta.SrcTags = e.resolveServiceTags(meta.SrcContainerID)
		}
		// Resolve tags for destination container
		if meta.DstContainerID != "" {
			meta.DstService, meta.DstTags = e.resolveServiceTags(meta.DstContainerID)
		}

		// Skip connections with no useful metadata
		if meta.SrcContainerID == "" && meta.DstContainerID == "" {
			continue
		}

		srcPort := uint16(conn.Laddr.Port)
		dstPort := uint16(conn.Raddr.Port)

		// Exact 5-tuple
		exactKey := connKey{
			SrcIP:    srcIP,
			DstIP:    dstIP,
			SrcPort:  srcPort,
			DstPort:  dstPort,
			Protocol: proto,
		}
		exact[exactKey] = meta

		// Wildcard with SrcPort=0 (for ephemeral source port rollup)
		noSrcKey := connKey{
			SrcIP:    srcIP,
			DstIP:    dstIP,
			SrcPort:  0,
			DstPort:  dstPort,
			Protocol: proto,
		}
		noSrc[noSrcKey] = meta

		// Wildcard with DstPort=0 (for ephemeral dest port rollup)
		noDstKey := connKey{
			SrcIP:    srcIP,
			DstIP:    dstIP,
			SrcPort:  srcPort,
			DstPort:  0,
			Protocol: proto,
		}
		noDst[noDstKey] = meta
	}

	// Swap under write lock
	e.mu.Lock()
	e.exactIndex = exact
	e.noSrcPort = noSrc
	e.noDstPort = noDst
	e.mu.Unlock()
}

func (e *ConnEnricher) resolveServiceTags(containerID string) (string, []string) {
	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	tags, err := e.tagger.Tag(entityID, taggertypes.LowCardinality)
	if err != nil {
		e.logger.Debugf("ConnEnricher: failed to get tags for container %s: %v", containerID, err)
		return "", nil
	}

	service := ""
	for _, tag := range tags {
		if strings.HasPrefix(tag, "service:") {
			service = tag[len("service:"):]
			break
		}
	}
	return service, tags
}

// Lookup looks up eBPF connection metadata for a given flow tuple.
// srcPort/dstPort use -1 to indicate ephemeral port rollup.
func (e *ConnEnricher) Lookup(srcIP, dstIP []byte, srcPort, dstPort int32, ipProtocol uint32) *connMeta {
	e.mu.RLock()
	defer e.mu.RUnlock()

	normalizedSrc := normalizeIPBytes(srcIP)
	normalizedDst := normalizeIPBytes(dstIP)
	proto := uint8(ipProtocol)

	// Try exact match first (when neither port is ephemeral)
	if srcPort >= 0 && dstPort >= 0 {
		key := connKey{
			SrcIP:    normalizedSrc,
			DstIP:    normalizedDst,
			SrcPort:  uint16(srcPort),
			DstPort:  uint16(dstPort),
			Protocol: proto,
		}
		if meta, ok := e.exactIndex[key]; ok {
			return meta
		}
	}

	// Ephemeral source port (-1): look up with SrcPort=0
	if srcPort == -1 {
		key := connKey{
			SrcIP:    normalizedSrc,
			DstIP:    normalizedDst,
			SrcPort:  0,
			DstPort:  uint16(dstPort),
			Protocol: proto,
		}
		if meta, ok := e.noSrcPort[key]; ok {
			return meta
		}
	}

	// Ephemeral dest port (-1): look up with DstPort=0
	if dstPort == -1 {
		key := connKey{
			SrcIP:    normalizedSrc,
			DstIP:    normalizedDst,
			SrcPort:  uint16(srcPort),
			DstPort:  0,
			Protocol: proto,
		}
		if meta, ok := e.noDstPort[key]; ok {
			return meta
		}
	}

	return nil
}

// normalizeIP parses an IP string and returns a 16-byte representation.
func normalizeIP(ipStr string) [16]byte {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return [16]byte{}
	}
	ip = ip.To16()
	if ip == nil {
		return [16]byte{}
	}
	var result [16]byte
	copy(result[:], ip)
	return result
}

// normalizeIPBytes converts a raw IP byte slice to a 16-byte representation.
func normalizeIPBytes(ipBytes []byte) [16]byte {
	var result [16]byte
	switch len(ipBytes) {
	case 4:
		// IPv4 → v4-mapped-v6
		result[10] = 0xff
		result[11] = 0xff
		copy(result[12:], ipBytes)
	case 16:
		copy(result[:], ipBytes)
	}
	return result
}

// connectionTypeToProtocol converts model.ConnectionType to an IP protocol number.
func connectionTypeToProtocol(connType model.ConnectionType) uint8 {
	switch connType {
	case model.ConnectionType_tcp:
		return 6 // TCP
	case model.ConnectionType_udp:
		return 17 // UDP
	default:
		return 0
	}
}

// directionString converts model.ConnectionDirection to a human-readable string.
func directionString(dir model.ConnectionDirection) string {
	switch dir {
	case model.ConnectionDirection_incoming:
		return "incoming"
	case model.ConnectionDirection_outgoing:
		return "outgoing"
	case model.ConnectionDirection_local:
		return "local"
	default:
		return "unspecified"
	}
}

// addConnEnrichment enriches a flow with eBPF connection metadata.
func addConnEnrichment(flow *common.Flow, enricher *ConnEnricher) {
	if enricher == nil {
		return
	}

	meta := enricher.Lookup(flow.SrcAddr, flow.DstAddr, flow.SrcPort, flow.DstPort, flow.IPProtocol)
	if meta == nil {
		return
	}

	if flow.AdditionalFields == nil {
		flow.AdditionalFields = make(common.AdditionalFields)
	}

	flow.AdditionalFields["ebpf.matched"] = true
	if meta.SrcService != "" {
		flow.AdditionalFields["ebpf.src.service"] = meta.SrcService
	}
	if meta.DstService != "" {
		flow.AdditionalFields["ebpf.dst.service"] = meta.DstService
	}
	if meta.SrcContainerID != "" {
		flow.AdditionalFields["ebpf.src.container_id"] = meta.SrcContainerID
	}
	if meta.DstContainerID != "" {
		flow.AdditionalFields["ebpf.dst.container_id"] = meta.DstContainerID
	}
	if len(meta.SrcTags) > 0 {
		flow.AdditionalFields["ebpf.src.tags"] = strings.Join(meta.SrcTags, ",")
	}
	if len(meta.DstTags) > 0 {
		flow.AdditionalFields["ebpf.dst.tags"] = strings.Join(meta.DstTags, ",")
	}
	if meta.Direction != "" {
		flow.AdditionalFields["ebpf.direction"] = meta.Direction
	}
}
