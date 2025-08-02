// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/detector"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathServices = "/services"
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// discovery is an implementation of the Module interface for the discovery module.
type discovery struct {
	core core.Discovery

	config *core.DiscoveryConfig

	mux *sync.RWMutex

	// privilegedDetector is used to detect the language of a process.
	privilegedDetector privileged.LanguageDetector

	// scrubber is used to remove potentially sensitive data from the command line
	scrubber *procutil.DataScrubber
}

type networkCollectorFactory func(_ *core.DiscoveryConfig) (core.NetworkCollector, error)

func newDiscoveryWithNetwork(getNetworkCollector networkCollectorFactory) *discovery {
	cfg := core.NewConfig()

	var network core.NetworkCollector
	if cfg.NetworkStatsEnabled {
		var err error
		network, err = getNetworkCollector(cfg)
		if err != nil {
			log.Warn("unable to get network collector", err)

			// Do not fail on error since the collector could fail due to eBPF
			// errors but we want the rest of our module to continue.
			network = nil
		}
	}

	return &discovery{
		core: core.Discovery{
			Config:  cfg,
			Network: network,
		},
		config:             cfg,
		mux:                &sync.RWMutex{},
		privilegedDetector: privileged.NewLanguageDetector(),
		scrubber:           procutil.NewDefaultDataScrubber(),
	}
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
	d := newDiscoveryWithNetwork(newNetworkCollector)

	return d, nil
}

// GetStats returns the stats of the discovery module.
func (s *discovery) GetStats() map[string]any {
	return nil
}

// Register registers the discovery module with the provided HTTP mux.
func (s *discovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/status", s.handleStatusEndpoint)
	httpMux.HandleFunc("/state", s.handleStateEndpoint)
	httpMux.HandleFunc("/network-stats", s.handleNetworkStatsEndpoint)
	httpMux.HandleFunc(pathServices, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))

	return nil
}

// Close cleans resources used by the discovery module.
func (s *discovery) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.core.Close()
}

// handleStatusEndpoint is the handler for the /status endpoint.
// Reports the status of the discovery module.
func (s *discovery) handleStatusEndpoint(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("Discovery Module is running"))
}

type state struct {
	NetworkEnabled bool `json:"network_enabled"`
}

// handleStateEndpoint is the handler for the /state endpoint.
// Returns the internal state of the discovery module.
func (s *discovery) handleStateEndpoint(w http.ResponseWriter, _ *http.Request) {
	s.mux.Lock()
	defer s.mux.Unlock()

	state := &state{
		NetworkEnabled: s.core.Network != nil,
	}

	utils.WriteAsJSON(w, state, utils.CompactOutput)
}

func (s *discovery) handleServices(w http.ResponseWriter, req *http.Request) {
	params, err := core.ParseParamsFromRequest(req)
	if err != nil {
		_ = log.Errorf("invalid params to /discovery%s: %v", pathServices, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	services, err := s.getServices(params)
	if err != nil {
		_ = log.Errorf("failed to handle /discovery%s: %v", pathServices, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	utils.WriteAsJSON(w, services, utils.CompactOutput)
}

// socketInfo stores information related to each socket.
type socketInfo struct {
	port uint16
}

// namespaceInfo stores information related to each network namespace.
type namespaceInfo struct {
	// listeningSockets maps socket inode numbers to socket information for listening sockets.
	listeningSockets map[uint64]socketInfo
}

// Lifted from pkg/network/proc_net.go
const (
	tcpListen uint64 = 10

	// tcpClose is also used to indicate a UDP connection where the other end hasn't been established
	tcpClose  uint64 = 7
	udpListen        = tcpClose
)

const (
	// readLimit is used by io.LimitReader while reading the content of the
	// /proc/net/udp{,6} files. The number of lines inside such a file is dynamic
	// as each line represents a single used socket.
	// In theory, the number of available sockets is 65535 (2^16 - 1) per IP.
	// With e.g. 150 Byte per line and the maximum number of 65535,
	// the reader needs to handle 150 Byte * 65535 =~ 10 MB for a single IP.
	// Taken from net_ip_socket.go from github.com/prometheus/procfs.
	readLimit = 4294967296 // Byte -> 4 GiB
)

var (
	errInvalidLine      = errors.New("invalid line")
	errInvalidState     = errors.New("invalid state field")
	errUnsupportedState = errors.New("unsupported state field")
	errInvalidLocalIP   = errors.New("invalid local ip format")
	errInvalidLocalPort = errors.New("invalid local port format")
	errInvalidInode     = errors.New("invalid inode format")
)

// parseNetIPSocketLine parses a single line, represented by a list of fields.
// It returns the inode and local port of the socket if the line is valid.
// Based on parseNetIPSocketLine() in net_ip_socket.go from github.com/prometheus/procfs.
func parseNetIPSocketLine(fields []string, expectedState uint64) (uint64, uint16, error) {
	if len(fields) < 10 {
		return 0, 0, errInvalidLine
	}
	var localPort uint16
	var inode uint64

	if state, err := strconv.ParseUint(fields[3], 16, 64); err != nil {
		return 0, 0, errInvalidState
	} else if state != expectedState {
		return 0, 0, errUnsupportedState
	}

	// local_address
	l := strings.Split(fields[1], ":")
	if len(l) != 2 {
		return 0, 0, errInvalidLocalIP
	}
	localPortTemp, err := strconv.ParseUint(l[1], 16, 64)
	if err != nil {
		return 0, 0, errInvalidLocalPort
	}
	localPort = uint16(localPortTemp)

	if inode, err = strconv.ParseUint(fields[9], 0, 64); err != nil {
		return 0, 0, errInvalidInode
	}

	return inode, localPort, nil
}

// newNetIPSocket reads the content of the provided file and returns a map of socket inodes to ports.
// Based on newNetIPSocket() in net_ip_socket.go from github.com/prometheus/procfs
func newNetIPSocket(file string, expectedState uint64, shouldIgnore func(uint16) bool) (map[uint64]uint16, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	netIPSocket := make(map[uint64]uint16)

	lr := io.LimitReader(f, readLimit)
	s := bufio.NewScanner(lr)
	s.Scan() // skip first line with headers
	for s.Scan() {
		fields := strings.Fields(s.Text())
		inode, port, err := parseNetIPSocketLine(fields, expectedState)
		if err != nil {
			continue
		}

		if shouldIgnore != nil && shouldIgnore(port) {
			continue
		}

		netIPSocket[inode] = port
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return netIPSocket, nil
}

// getNsInfo gets the list of open ports with socket inodes for all supported
// protocols for the provided namespace. Based on snapshotBoundSockets() in
// pkg/security/security_profile/activity_tree/process_node_snapshot.go.
func getNsInfo(pid int) (*namespaceInfo, error) {
	// Don't ignore ephemeral ports on TCP, unlike on UDP (see below).
	var noIgnore func(uint16) bool
	tcp, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/tcp", pid)), tcpListen, noIgnore)
	if err != nil {
		log.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	udp, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/udp", pid)), udpListen,
		func(port uint16) bool {
			// As in NPM (see initializePortBind() in
			// pkg/network/tracer/connection): Ignore ephemeral port binds on
			// UDP as they are more likely to be from clients calling bind with
			// port 0.
			return network.IsPortInEphemeralRange(network.AFINET, network.UDP, port) == network.EphemeralTrue
		})
	if err != nil {
		log.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	tcpv6, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/tcp6", pid)), tcpListen, noIgnore)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// The file won't exist if IPv6 is disabled.
		log.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	udpv6, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/udp6", pid)), udpListen,
		func(port uint16) bool {
			return network.IsPortInEphemeralRange(network.AFINET6, network.UDP, port) == network.EphemeralTrue
		})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Debugf("couldn't snapshot UDP6 sockets: %v", err)
	}

	listeningSockets := make(map[uint64]socketInfo, len(tcp)+len(udp)+len(tcpv6)+len(udpv6))
	for _, mmap := range []map[uint64]uint16{tcp, udp, tcpv6, udpv6} {
		for inode, info := range mmap {
			listeningSockets[inode] = socketInfo{
				port: info,
			}
		}
	}
	return &namespaceInfo{
		listeningSockets: listeningSockets,
	}, nil
}

// parsingContext holds temporary context not preserved between invocations of
// the endpoint.
type parsingContext struct {
	procRoot       string
	netNsInfo      map[uint32]*namespaceInfo
	readlinkBuffer []byte
}

func newParsingContext() parsingContext {
	return parsingContext{
		procRoot:       kernel.ProcFSRoot(),
		netNsInfo:      make(map[uint32]*namespaceInfo),
		readlinkBuffer: make([]byte, readlinkBufferSize),
	}
}

// getServiceInfo gets the service information for a process using the
// servicedetector module.
func (s *discovery) getServiceInfo(pid int32) (*model.Service, error) {
	proc := &process.Process{
		Pid: pid,
	}

	cmdline, err := proc.CmdlineSlice()
	if err != nil {
		return nil, err
	}

	exe, err := proc.Exe()
	if err != nil {
		return nil, err
	}

	var tracerMetadataArr []tracermetadata.TracerMetadata
	var firstMetadata *tracermetadata.TracerMetadata

	tracerMetadata, err := tracermetadata.GetTracerMetadata(int(pid), kernel.ProcFSRoot())
	if err == nil {
		// Currently we only get the first tracer metadata
		tracerMetadataArr = append(tracerMetadataArr, tracerMetadata)
		firstMetadata = &tracerMetadata
	}

	root := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "root")
	lang := language.Detect(exe, cmdline, proc.Pid, s.privilegedDetector, firstMetadata)
	env, err := getTargetEnvs(proc)
	if err != nil {
		return nil, err
	}

	contextMap := make(usm.DetectorContextMap)
	contextMap[usm.ServiceProc] = proc

	fs := usm.NewSubDirFS(root)
	ctx := usm.NewDetectionContext(cmdline, env, fs)
	ctx.Pid = int(proc.Pid)
	ctx.ContextMap = contextMap

	nameMeta := detector.GetServiceName(lang, ctx)
	apmInstrumentation := apm.Detect(lang, ctx, firstMetadata)

	cmdline, _ = s.scrubber.ScrubCommand(cmdline)

	return &model.Service{
		PID:                      int(pid),
		GeneratedName:            nameMeta.Name,
		GeneratedNameSource:      string(nameMeta.Source),
		AdditionalGeneratedNames: nameMeta.AdditionalNames,
		DDService:                nameMeta.DDService,
		TracerMetadata:           tracerMetadataArr,
		Language:                 string(lang),
		APMInstrumentation:       string(apmInstrumentation),
		CommandLine:              truncateCmdline(lang, cmdline),
	}, nil
}

// maxNumberOfPorts is the maximum number of listening ports which we report per
// service.
const maxNumberOfPorts = 50

func (s *discovery) getPorts(context parsingContext, pid int32, sockets []uint64) ([]uint16, error) {
	if len(sockets) == 0 {
		return nil, nil
	}

	ns, err := netns.GetNetNsInoFromPid(context.procRoot, int(pid))
	if err != nil {
		return nil, err
	}

	// The socket and network address information are different for each
	// network namespace.  Since namespaces can be shared between multiple
	// processes, we cache them to only parse them once per call to this
	// function.
	nsInfo, ok := context.netNsInfo[ns]
	if !ok {
		nsInfo, err = getNsInfo(int(pid))
		if err != nil {
			return nil, err
		}

		context.netNsInfo[ns] = nsInfo
	}

	var ports []uint16
	seenPorts := make(map[uint16]struct{})
	for _, socket := range sockets {
		if info, ok := nsInfo.listeningSockets[socket]; ok {
			port := info.port
			if _, seen := seenPorts[port]; seen {
				continue
			}

			ports = append(ports, port)
			seenPorts[port] = struct{}{}
		}
	}

	if len(ports) == 0 {
		return nil, nil
	}

	if len(ports) > maxNumberOfPorts {
		// Sort the list so that non-ephemeral ports are given preference when
		// we trim the list.
		portCmp := func(a, b uint16) int {
			return cmp.Compare(a, b)
		}
		slices.SortFunc(ports, portCmp)
		ports = ports[:maxNumberOfPorts]
	}

	return ports, nil
}

// getServices processes a list of PIDs and returns service information for each.
// This is used by the /services endpoint which accepts explicit PID lists and bypasses
// the port retry logic used by the /check endpoint. The caller (the Core-Agent
// process collector) will handle the retry..
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	response := &model.ServicesResponse{
		Services: make([]model.Service, 0),
	}

	context := newParsingContext()

	for _, pid := range params.Pids {
		service := s.getServiceWithoutRetry(context, int32(pid))
		if service == nil {
			continue
		}
		response.Services = append(response.Services, *service)
	}

	return response, nil
}

// getServiceWithoutRetry extracts service information for a PID without port retry logic.
// Unlike getService(), this function immediately returns nil if no ports are found,
// rather than tracking retry attempts. This is used by the /services endpoint.
func (s *discovery) getServiceWithoutRetry(context parsingContext, pid int32) *model.Service {
	if s.shouldIgnoreComm(pid) {
		return nil
	}

	openFileInfo, err := getOpenFilesInfo(pid, context.readlinkBuffer)
	if err != nil {
		return nil
	}
	ports, err := s.getPorts(context, pid, openFileInfo.sockets)
	if err != nil {
		return nil
	}
	if len(ports) == 0 {
		return nil
	}

	service, err := s.getServiceInfo(pid)
	if err != nil {
		log.Tracef("[pid: %d] could not get service info: %v", pid, err)
		return nil
	}

	service.Ports = ports
	service.LogFiles = getLogFiles(pid, openFileInfo.logs)
	service.Type = string(servicetype.Detect(ports))

	return service
}

// handleNetworkStatsEndpoint is the handler for the /network-stats endpoint.
// Returns network statistics for the provided list of PIDs.
func (s *discovery) handleNetworkStatsEndpoint(w http.ResponseWriter, req *http.Request) {
	if s.core.Network == nil {
		http.Error(w, "network stats collection is not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse PIDs from query parameter
	pidsStr := req.URL.Query().Get("pids")
	if pidsStr == "" {
		http.Error(w, "missing required 'pids' query parameter", http.StatusBadRequest)
		return
	}

	// Split and parse PIDs
	pidStrs := strings.Split(pidsStr, ",")
	pids := make(core.PidSet, len(pidStrs))
	for _, pidStr := range pidStrs {
		pid, err := strconv.ParseInt(pidStr, 10, 32)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid PID format: %s", pidStr), http.StatusBadRequest)
			return
		}
		pids.Add(int32(pid))
	}

	// Get network stats
	stats, err := s.core.Network.GetStats(pids)
	if err != nil {
		log.Errorf("failed to get network stats: %v", err)
		http.Error(w, "failed to get network stats", http.StatusInternalServerError)
		return
	}

	// Convert stats to response format
	response := model.NetworkStatsResponse{
		Stats: make(map[int]model.NetworkStats, len(stats)),
	}
	for pid, stat := range stats {
		response.Stats[int(pid)] = model.NetworkStats{
			RxBytes: stat.Rx,
			TxBytes: stat.Tx,
		}
	}

	utils.WriteAsJSON(w, response, utils.CompactOutput)
}
