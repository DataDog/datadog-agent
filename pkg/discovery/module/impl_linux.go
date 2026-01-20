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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/discovery/apm"
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathServices    = "/services"
	pathConnections = "/connections"
)

var (
	// apmInjectorRegex matches the APM auto-injector launcher.preload.so library path
	apmInjectorRegex = regexp.MustCompile(`/opt/datadog-packages/datadog-apm-inject/[^/]+/inject/launcher\.preload\.so`)
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
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
	cfg := core.NewConfig()

	d := &discovery{
		core: core.Discovery{
			Config: cfg,
		},
		config:             cfg,
		mux:                &sync.RWMutex{},
		privilegedDetector: privileged.NewLanguageDetector(),
	}

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
	httpMux.HandleFunc(pathServices, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))
	httpMux.HandleFunc(pathConnections, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleConnections))

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

// handleStateEndpoint is the handler for the /state endpoint.
// Returns the internal state of the discovery module.
func (s *discovery) handleStateEndpoint(w http.ResponseWriter, _ *http.Request) {
	s.mux.Lock()
	defer s.mux.Unlock()

	state := make(map[string]interface{})

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
	// tcpSockets maps socket inode numbers to socket information for listening TCP sockets.
	tcpSockets map[uint64]socketInfo
	// udpSockets maps socket inode numbers to socket information for listening UDP sockets.
	udpSockets map[uint64]socketInfo
}

// Lifted from pkg/network/proc_net.go
const (
	tcpEstablished uint64 = 1
	tcpListen      uint64 = 10

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

	tcpSockets := make(map[uint64]socketInfo, len(tcp)+len(tcpv6))
	udpSockets := make(map[uint64]socketInfo, len(udp)+len(udpv6))

	for _, ports := range []map[uint64]uint16{tcp, tcpv6} {
		for inode, port := range ports {
			tcpSockets[inode] = socketInfo{
				port: port,
			}
		}
	}

	for _, ports := range []map[uint64]uint16{udp, udpv6} {
		for inode, port := range ports {
			udpSockets[inode] = socketInfo{
				port: port,
			}
		}
	}

	return &namespaceInfo{
		tcpSockets: tcpSockets,
		udpSockets: udpSockets,
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
func (s *discovery) getServiceInfo(pid int32, openFiles openFilesInfo) (*model.Service, error) {
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

	if openFiles.tracerMemfdFd != "" {
		fdPath := kernel.HostProc(strconv.Itoa(int(pid)), "fd", openFiles.tracerMemfdFd)
		tracerMetadata, err := tracermetadata.GetTracerMetadataFromPath(fdPath)
		if err == nil {
			// Currently we only get the first tracer metadata
			tracerMetadataArr = append(tracerMetadataArr, tracerMetadata)
			firstMetadata = &tracerMetadata
		}
	}

	root := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "root")
	lang := language.Detect(exe, cmdline, proc.Pid, s.privilegedDetector, firstMetadata)
	env, err := GetTargetEnvs(proc)
	if err != nil {
		return nil, err
	}

	contextMap := make(usm.DetectorContextMap)
	contextMap[usm.ServiceProc] = proc

	fs := usm.NewSubDirFS(root)
	ctx := usm.NewDetectionContext(cmdline, env, fs)
	ctx.Pid = int(proc.Pid)
	ctx.ContextMap = contextMap

	nameMeta, _ := usm.ExtractServiceMetadata(lang, ctx)
	apmInstrumentation := apm.Detect(lang, ctx, firstMetadata)

	return &model.Service{
		PID:                      int(pid),
		GeneratedName:            nameMeta.Name,
		GeneratedNameSource:      string(nameMeta.Source),
		AdditionalGeneratedNames: nameMeta.AdditionalNames,
		TracerMetadata:           tracerMetadataArr,
		UST: model.UST{
			Service: env.GetDefault("DD_SERVICE", ""),
			Env:     env.GetDefault("DD_ENV", ""),
			Version: env.GetDefault("DD_VERSION", ""),
		},
		Language:           string(lang),
		APMInstrumentation: apmInstrumentation == apm.Provided,
	}, nil
}

// getHeartbeatServiceInfo gets minimal service information for heartbeat processes.
// This only collects dynamic fields (ports and log files) and skips expensive operations
// like language detection, service name generation, and APM instrumentation detection.
func (s *discovery) getHeartbeatServiceInfo(context parsingContext, pid int32) *model.Service {
	if s.shouldIgnoreComm(pid) {
		return nil
	}

	openFileInfo, err := getOpenFilesInfo(pid, context.readlinkBuffer)
	if err != nil {
		return nil
	}
	tcpPorts, udpPorts, err := s.getPorts(context, pid, openFileInfo.sockets)
	if err != nil {
		return nil
	}

	totalPorts := len(tcpPorts) + len(udpPorts)
	hasTracerMetadata := openFileInfo.tracerMemfdFd != ""
	hasLogs := len(openFileInfo.logs) > 0
	if totalPorts == 0 && !hasTracerMetadata && !hasLogs {
		return nil
	}

	logFiles := getLogFiles(pid, openFileInfo.logs)

	// Return minimal service info with only dynamic fields
	return &model.Service{
		PID:      int(pid),
		TCPPorts: tcpPorts,
		UDPPorts: udpPorts,
		LogFiles: logFiles,
	}
}

// maxNumberOfPorts is the maximum number of listening ports which we report per
// service.
const maxNumberOfPorts = 50

// getPorts gets the list of open ports for the provided process.
func (s *discovery) getPorts(context parsingContext, pid int32, sockets []uint64) ([]uint16, []uint16, error) {
	if len(sockets) == 0 {
		return nil, nil, nil
	}

	ns, err := netns.GetNetNsInoFromPid(context.procRoot, int(pid))
	if err != nil {
		return nil, nil, err
	}

	// The socket and network address information are different for each
	// network namespace.  Since namespaces can be shared between multiple
	// processes, we cache them to only parse them once per call to this
	// function.
	nsInfo, ok := context.netNsInfo[ns]
	if !ok {
		nsInfo, err = getNsInfo(int(pid))
		if err != nil {
			return nil, nil, err
		}

		context.netNsInfo[ns] = nsInfo
	}

	var tcpPorts, udpPorts []uint16
	seenTCPPorts := make(map[uint16]struct{})
	seenUDPPorts := make(map[uint16]struct{})

	for _, socket := range sockets {
		if info, ok := nsInfo.tcpSockets[socket]; ok {
			port := info.port
			if _, seen := seenTCPPorts[port]; seen {
				continue
			}
			tcpPorts = append(tcpPorts, port)
			seenTCPPorts[port] = struct{}{}
			continue
		}

		if info, ok := nsInfo.udpSockets[socket]; ok {
			port := info.port
			if _, seen := seenUDPPorts[port]; seen {
				continue
			}
			udpPorts = append(udpPorts, port)
			seenUDPPorts[port] = struct{}{}
			continue
		}
	}

	// Sort the list so that non-ephemeral ports are given preference when we
	// trim the list.
	portCmp := func(a, b uint16) int {
		return cmp.Compare(a, b)
	}
	if len(tcpPorts) > maxNumberOfPorts {
		slices.SortFunc(tcpPorts, portCmp)
		tcpPorts = tcpPorts[:maxNumberOfPorts]
	}

	if len(udpPorts) > maxNumberOfPorts {
		slices.SortFunc(udpPorts, portCmp)
		udpPorts = udpPorts[:maxNumberOfPorts]
	}

	return tcpPorts, udpPorts, nil
}

// getServices processes categorized PID lists and returns service information for each.
// This is used by the /services endpoint which accepts explicit PID lists and bypasses
// the port retry logic used by the /check endpoint. The caller (the Core-Agent
// process collector) will handle the retry.
func (s *discovery) getServices(params core.Params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	response := &model.ServicesResponse{
		Services: make([]model.Service, 0),
	}

	context := newParsingContext()

	// Process new PIDs with full service info collection
	for _, pid := range params.NewPids {
		// Check for APM injector even if process is not detected as a service
		if detectAPMInjectorFromMaps(pid) {
			response.InjectedPIDs = append(response.InjectedPIDs, int(pid))
		}

		service := s.getServiceWithoutRetry(context, pid)
		if service == nil {
			continue
		}
		response.Services = append(response.Services, *service)
	}

	// Process heartbeat PIDs with minimal updates (only ports and log files)
	for _, pid := range params.HeartbeatPids {
		service := s.getHeartbeatServiceInfo(context, pid)
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
	tcpPorts, udpPorts, err := s.getPorts(context, pid, openFileInfo.sockets)
	if err != nil {
		return nil
	}

	totalPorts := len(tcpPorts) + len(udpPorts)
	hasTracerMetadata := openFileInfo.tracerMemfdFd != ""
	hasLogs := len(openFileInfo.logs) > 0
	if totalPorts == 0 && !hasTracerMetadata && !hasLogs {
		return nil
	}

	service, err := s.getServiceInfo(pid, openFileInfo)
	if err != nil {
		log.Tracef("[pid: %d] could not get service info: %v", pid, err)
		return nil
	}

	service.TCPPorts = tcpPorts
	service.UDPPorts = udpPorts
	service.LogFiles = getLogFiles(pid, openFileInfo.logs)
	service.Type = string(servicetype.Detect(tcpPorts, udpPorts))

	return service
}

// detectAPMInjectorFromMaps reads /proc/pid/maps and checks for APM injector library
func detectAPMInjectorFromMaps(pid int32) bool {
	mapsPath := kernel.HostProc(strconv.Itoa(int(pid)), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return false
	}
	defer mapsFile.Close()

	return detectAPMInjectorFromMapsReader(mapsFile)
}

// detectAPMInjectorFromMapsReader checks for APM injector library in the provided reader
func detectAPMInjectorFromMapsReader(reader io.Reader) bool {
	lr := io.LimitReader(reader, readLimit)
	scanner := bufio.NewScanner(lr)
	for scanner.Scan() {
		line := scanner.Text()
		if apmInjectorRegex.MatchString(line) {
			return true
		}
	}

	return false
}

// handleConnections handles the /discovery/connections endpoint.
func (s *discovery) handleConnections(w http.ResponseWriter, _ *http.Request) {
	connections, err := s.getConnections()
	if err != nil {
		_ = log.Errorf("failed to handle /discovery%s: %v", pathConnections, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	utils.WriteAsJSON(w, connections, utils.CompactOutput)
}

// establishedConnInfo stores information about an established TCP connection.
type establishedConnInfo struct {
	localIP    string
	localPort  uint16
	remoteIP   string
	remotePort uint16
	family     string // "v4" or "v6"
}

// parseEstablishedConnLine parses a single line from /proc/net/tcp{,6} for ESTABLISHED connections.
// It returns connection info including local/remote addresses.
func parseEstablishedConnLine(fields []string, family string) (*establishedConnInfo, uint64, error) {
	if len(fields) < 10 {
		return nil, 0, errInvalidLine
	}

	state, err := strconv.ParseUint(fields[3], 16, 64)
	if err != nil {
		return nil, 0, errInvalidState
	}
	if state != tcpEstablished {
		return nil, 0, errUnsupportedState
	}

	// Parse local address (fields[1] is "IP:PORT")
	localParts := strings.Split(fields[1], ":")
	if len(localParts) != 2 {
		return nil, 0, errInvalidLocalIP
	}
	localIP, err := parseHexIP(localParts[0], family)
	if err != nil {
		return nil, 0, errInvalidLocalIP
	}
	localPort, err := strconv.ParseUint(localParts[1], 16, 16)
	if err != nil {
		return nil, 0, errInvalidLocalPort
	}

	// Parse remote address (fields[2] is "IP:PORT")
	remoteParts := strings.Split(fields[2], ":")
	if len(remoteParts) != 2 {
		return nil, 0, errors.New("invalid remote ip format")
	}
	remoteIP, err := parseHexIP(remoteParts[0], family)
	if err != nil {
		return nil, 0, errors.New("invalid remote ip format")
	}
	remotePort, err := strconv.ParseUint(remoteParts[1], 16, 16)
	if err != nil {
		return nil, 0, errors.New("invalid remote port format")
	}

	inode, err := strconv.ParseUint(fields[9], 0, 64)
	if err != nil {
		return nil, 0, errInvalidInode
	}

	return &establishedConnInfo{
		localIP:    localIP,
		localPort:  uint16(localPort),
		remoteIP:   remoteIP,
		remotePort: uint16(remotePort),
		family:     family,
	}, inode, nil
}

// parseHexIP converts a hexadecimal IP address from /proc/net/tcp{,6} to a human-readable string.
func parseHexIP(hexIP string, family string) (string, error) {
	if family == "v6" {
		// IPv6 is 32 hex chars (128 bits), stored in network byte order per 32-bit word
		if len(hexIP) != 32 {
			return "", errors.New("invalid IPv6 length")
		}
		// IPv6 in /proc/net/tcp6 is stored as 4 little-endian 32-bit words
		var parts [8]uint16
		for i := 0; i < 4; i++ {
			word := hexIP[i*8 : (i+1)*8]
			// Reverse byte order within each 32-bit word
			b0, _ := strconv.ParseUint(word[6:8], 16, 8)
			b1, _ := strconv.ParseUint(word[4:6], 16, 8)
			b2, _ := strconv.ParseUint(word[2:4], 16, 8)
			b3, _ := strconv.ParseUint(word[0:2], 16, 8)
			parts[i*2] = uint16(b0<<8 | b1)
			parts[i*2+1] = uint16(b2<<8 | b3)
		}
		return fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x",
			parts[0], parts[1], parts[2], parts[3],
			parts[4], parts[5], parts[6], parts[7]), nil
	}

	// IPv4 is 8 hex chars (32 bits), stored in little-endian
	if len(hexIP) != 8 {
		return "", errors.New("invalid IPv4 length")
	}
	b0, _ := strconv.ParseUint(hexIP[6:8], 16, 8)
	b1, _ := strconv.ParseUint(hexIP[4:6], 16, 8)
	b2, _ := strconv.ParseUint(hexIP[2:4], 16, 8)
	b3, _ := strconv.ParseUint(hexIP[0:2], 16, 8)
	return fmt.Sprintf("%d.%d.%d.%d", b0, b1, b2, b3), nil
}

// getEstablishedConnections reads established TCP connections from /proc/net/tcp{,6}.
// Returns a map of inode -> connection info.
func getEstablishedConnections(pid int, family string) (map[uint64]*establishedConnInfo, error) {
	var filename string
	if family == "v6" {
		filename = kernel.HostProc(fmt.Sprintf("%d/net/tcp6", pid))
	} else {
		filename = kernel.HostProc(fmt.Sprintf("%d/net/tcp", pid))
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	connections := make(map[uint64]*establishedConnInfo)
	lr := io.LimitReader(f, readLimit)
	scanner := bufio.NewScanner(lr)
	scanner.Scan() // skip header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		connInfo, inode, err := parseEstablishedConnLine(fields, family)
		if err != nil {
			continue
		}
		connections[inode] = connInfo
	}

	return connections, scanner.Err()
}

// getConnections collects all established TCP connections from all processes.
func (s *discovery) getConnections() (*model.ConnectionsResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	// Get the list of listening ports to determine direction
	context := newParsingContext()
	listeningPorts := make(map[uint16]struct{})

	// We need to scan processes to find all sockets and their owners
	procDir, err := os.Open(kernel.HostProc())
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc: %w", err)
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc entries: %w", err)
	}

	var connections []model.Connection

	// Track seen connections to avoid duplicates (same connection from different process views)
	type connKey struct {
		localIP    string
		localPort  uint16
		remoteIP   string
		remotePort uint16
	}
	seenConns := make(map[connKey]struct{})

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue // not a PID directory
		}

		// Get network namespace info (for listening ports)
		ns, err := netns.GetNetNsInoFromPid(context.procRoot, pid)
		if err != nil {
			continue
		}

		// Cache listening ports per namespace
		if _, ok := context.netNsInfo[ns]; !ok {
			nsInfo, err := getNsInfo(pid)
			if err != nil {
				continue
			}
			context.netNsInfo[ns] = nsInfo
			// Add listening ports to the global set
			for _, info := range nsInfo.tcpSockets {
				listeningPorts[info.port] = struct{}{}
			}
		}

		// Get established connections for IPv4 and IPv6
		for _, family := range []string{"v4", "v6"} {
			establishedConns, err := getEstablishedConnections(pid, family)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) && family == "v6" {
					// IPv6 may be disabled
					continue
				}
				log.Debugf("couldn't get established connections for pid %d family %s: %v", pid, family, err)
				continue
			}

			// Get the socket inodes owned by this process
			openFileInfo, err := getOpenFilesInfo(int32(pid), context.readlinkBuffer)
			if err != nil {
				continue
			}

			// Match socket inodes to established connections
			for _, socketInode := range openFileInfo.sockets {
				connInfo, ok := establishedConns[socketInode]
				if !ok {
					continue
				}

				// Skip if we've already seen this connection
				key := connKey{
					localIP:    connInfo.localIP,
					localPort:  connInfo.localPort,
					remoteIP:   connInfo.remoteIP,
					remotePort: connInfo.remotePort,
				}
				if _, seen := seenConns[key]; seen {
					continue
				}
				seenConns[key] = struct{}{}

				// Determine direction based on whether local port is a listening port
				direction := "outgoing"
				if _, isListening := listeningPorts[connInfo.localPort]; isListening {
					direction = "incoming"
				}

				connections = append(connections, model.Connection{
					Laddr: model.Address{
						IP:   connInfo.localIP,
						Port: connInfo.localPort,
					},
					Raddr: model.Address{
						IP:   connInfo.remoteIP,
						Port: connInfo.remotePort,
					},
					Family:    connInfo.family,
					Type:      0, // TCP
					Direction: direction,
					PID:       uint32(pid),
					NetNS:     ns,
				})
			}
		}
	}

	return &model.ConnectionsResponse{
		Connections: connections,
	}, nil
}
