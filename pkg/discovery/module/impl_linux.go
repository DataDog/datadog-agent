// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/discovery/apm"
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/network"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
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

	// ciliumConntracker reads Cilium eBPF maps for NAT-translated connections.
	ciliumConntracker *ciliumConntracker
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

	if s.ciliumConntracker != nil {
		if err := s.ciliumConntracker.Close(); err != nil {
			log.Warnf("Failed to close Cilium conntracker: %v", err)
		}
	}

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
	procRoot             string
	netNsInfo            map[uint32]*namespaceInfo
	establishedConnsInfo map[uint32]*establishedConnsNsInfo
	readlinkBuffer       []byte
}

func newParsingContext() parsingContext {
	return parsingContext{
		procRoot:             kernel.ProcFSRoot(),
		netNsInfo:            make(map[uint32]*namespaceInfo),
		establishedConnsInfo: make(map[uint32]*establishedConnsNsInfo),
		readlinkBuffer:       make([]byte, readlinkBufferSize),
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

// establishedConnsNsInfo caches established connections per network namespace.
// Since /proc/<pid>/net/tcp{,6} is shared across all processes in the same
// network namespace, we only need to read it once per namespace.
type establishedConnsNsInfo struct {
	v4 map[uint64]*establishedConnInfo
	v6 map[uint64]*establishedConnInfo
}

// byteFieldIterator provides zero-allocation field iteration over a byte slice.
// It yields fields separated by whitespace without allocating new strings.
type byteFieldIterator struct {
	data []byte
}

// nextField returns the next whitespace-separated field from the data.
// Returns nil when no more fields are available.
func (iter *byteFieldIterator) nextField() []byte {
	// Skip any leading whitespace
	for len(iter.data) > 0 && iter.data[0] == ' ' {
		iter.data = iter.data[1:]
	}
	if len(iter.data) == 0 {
		return nil
	}

	// Read field up until the first whitespace char or end
	for i, b := range iter.data {
		if b == ' ' || b == '\n' {
			result := iter.data[:i]
			iter.data = iter.data[i:]
			return result
		}
	}

	// No whitespace found, return remainder
	result := iter.data
	iter.data = nil
	return result
}

// parseHexIPBytes converts a hexadecimal IP address from /proc/net/tcp{,6} to a human-readable string.
// This is a zero-allocation version that works with byte slices directly.
// Returns the IP string, the actual family (which may differ from input for IPv6-mapped IPv4), and an error.
func parseHexIPBytes(hexIP []byte, family string) (string, string, error) {
	if family == "v6" {
		// IPv6 is 32 hex chars (128 bits)
		if len(hexIP) != 32 {
			return "", "", errors.New("invalid IPv6 length")
		}
		// IPv6 in /proc/net/tcp6 is stored as 4 little-endian 32-bit words.
		var ipBytes [16]byte
		for i := 0; i < 4; i++ {
			word := hexIP[i*8 : (i+1)*8]
			// Reverse byte order within each 32-bit word
			b0 := hexByteToUint8(word[6], word[7])
			b1 := hexByteToUint8(word[4], word[5])
			b2 := hexByteToUint8(word[2], word[3])
			b3 := hexByteToUint8(word[0], word[1])
			ipBytes[i*4] = b0
			ipBytes[i*4+1] = b1
			ipBytes[i*4+2] = b2
			ipBytes[i*4+3] = b3
		}

		// Check if this is an IPv6-mapped IPv4 address (::ffff:x.x.x.x)
		if ipBytes[0] == 0 && ipBytes[1] == 0 && ipBytes[2] == 0 && ipBytes[3] == 0 &&
			ipBytes[4] == 0 && ipBytes[5] == 0 && ipBytes[6] == 0 && ipBytes[7] == 0 &&
			ipBytes[8] == 0 && ipBytes[9] == 0 && ipBytes[10] == 0xff && ipBytes[11] == 0xff {
			ip := net.IPv4(ipBytes[12], ipBytes[13], ipBytes[14], ipBytes[15])
			return ip.String(), "v4", nil
		}

		ip := net.IP(ipBytes[:])
		return ip.String(), "v6", nil
	}

	// IPv4 is 8 hex chars (32 bits), stored in little-endian
	if len(hexIP) != 8 {
		return "", "", errors.New("invalid IPv4 length")
	}
	b0 := hexByteToUint8(hexIP[6], hexIP[7])
	b1 := hexByteToUint8(hexIP[4], hexIP[5])
	b2 := hexByteToUint8(hexIP[2], hexIP[3])
	b3 := hexByteToUint8(hexIP[0], hexIP[1])
	ip := net.IPv4(b0, b1, b2, b3)
	return ip.String(), "v4", nil
}

// hexByteToUint8 converts two hex characters to a uint8.
// Assumes valid hex input (0-9, A-F, a-f).
func hexByteToUint8(high, low byte) byte {
	return hexCharToNibble(high)<<4 | hexCharToNibble(low)
}

// hexCharToNibble converts a hex character to its 4-bit value.
func hexCharToNibble(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	return 0
}

// parseHexPortBytes parses a hex port from a byte slice.
func parseHexPortBytes(b []byte) (uint16, error) {
	if len(b) == 0 || len(b) > 4 {
		return 0, errors.New("invalid port length")
	}
	var port uint16
	for _, c := range b {
		port = port<<4 | uint16(hexCharToNibble(c))
	}
	return port, nil
}

// parseEstablishedConnLineBytes parses a single line from /proc/net/tcp{,6} for ESTABLISHED connections.
// This is a zero-allocation version that works with byte slices directly.
func parseEstablishedConnLineBytes(line []byte, family string) (*establishedConnInfo, uint64, error) {
	iter := byteFieldIterator{data: line}

	// Field 0: sl (slot number, skip)
	if iter.nextField() == nil {
		return nil, 0, errInvalidLine
	}

	// Field 1: local_address (IP:PORT)
	localAddr := iter.nextField()
	if localAddr == nil {
		return nil, 0, errInvalidLine
	}

	// Field 2: rem_address (IP:PORT)
	remAddr := iter.nextField()
	if remAddr == nil {
		return nil, 0, errInvalidLine
	}

	// Field 3: st (state)
	stateField := iter.nextField()
	if stateField == nil {
		return nil, 0, errInvalidLine
	}

	// Parse state - only want ESTABLISHED (01)
	if len(stateField) != 2 || stateField[0] != '0' || stateField[1] != '1' {
		return nil, 0, errUnsupportedState
	}

	// Skip fields 4-8 (tx_queue:rx_queue, tr:tm->when, retrnsmt, uid, timeout)
	for i := 0; i < 5; i++ {
		if iter.nextField() == nil {
			return nil, 0, errInvalidLine
		}
	}

	// Field 9: inode
	inodeField := iter.nextField()
	if inodeField == nil {
		return nil, 0, errInvalidLine
	}

	// Parse local address
	colonIdx := bytes.IndexByte(localAddr, ':')
	if colonIdx == -1 {
		return nil, 0, errInvalidLocalIP
	}
	localIP, localFamily, err := parseHexIPBytes(localAddr[:colonIdx], family)
	if err != nil {
		return nil, 0, errInvalidLocalIP
	}
	localPort, err := parseHexPortBytes(localAddr[colonIdx+1:])
	if err != nil {
		return nil, 0, errInvalidLocalPort
	}

	// Parse remote address
	colonIdx = bytes.IndexByte(remAddr, ':')
	if colonIdx == -1 {
		return nil, 0, errors.New("invalid remote ip format")
	}
	remoteIP, remoteFamily, err := parseHexIPBytes(remAddr[:colonIdx], family)
	if err != nil {
		return nil, 0, errors.New("invalid remote ip format")
	}
	remotePort, err := parseHexPortBytes(remAddr[colonIdx+1:])
	if err != nil {
		return nil, 0, errors.New("invalid remote port format")
	}

	// Parse inode
	inode, err := parseUint64Bytes(inodeField)
	if err != nil {
		return nil, 0, errInvalidInode
	}

	// Use the local address family
	actualFamily := localFamily
	if localFamily != remoteFamily {
		log.Debugf("Family mismatch in connection: local=%s, remote=%s", localFamily, remoteFamily)
	}

	return &establishedConnInfo{
		localIP:    localIP,
		localPort:  localPort,
		remoteIP:   remoteIP,
		remotePort: remotePort,
		family:     actualFamily,
	}, inode, nil
}

// parseUint64Bytes parses a decimal uint64 from a byte slice.
func parseUint64Bytes(b []byte) (uint64, error) {
	if len(b) == 0 {
		return 0, errors.New("empty input")
	}
	var result uint64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid digit")
		}
		result = result*10 + uint64(c-'0')
	}
	return result, nil
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

	// Pre-size map to reduce rehashing (typical connection count)
	connections := make(map[uint64]*establishedConnInfo, 64)
	lr := io.LimitReader(f, readLimit)
	reader := bufio.NewReader(lr)

	// Skip header line
	_, err = reader.ReadSlice('\n')
	if err != nil {
		return connections, nil // empty file or just header
	}

	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			if err == io.EOF {
				// Process final line if it doesn't end with newline
				if len(line) > 0 {
					connInfo, inode, parseErr := parseEstablishedConnLineBytes(line, family)
					if parseErr == nil {
						connections[inode] = connInfo
					}
				}
				break
			}
			// For other errors, return what we have
			return connections, err
		}

		connInfo, inode, parseErr := parseEstablishedConnLineBytes(line, family)
		if parseErr != nil {
			continue
		}
		connections[inode] = connInfo
	}

	return connections, nil
}

// dockerProxyTarget represents the target address of a docker-proxy instance.
// Docker-proxy is used to forward traffic from host ports to container ports.
type dockerProxyTarget struct {
	containerIP   string
	containerPort uint16
	proxyIP       string // discovered from connections
}

// detectDockerProxy checks if a process is docker-proxy and extracts its target address.
// Docker-proxy command line looks like:
// /usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 32769 -container-ip 172.17.0.2 -container-port 6379
func detectDockerProxy(pid int) *dockerProxyTarget {
	cmdlinePath := kernel.HostProc(fmt.Sprintf("%d/cmdline", pid))
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return nil
	}

	// cmdline is null-separated
	args := strings.Split(string(data), "\x00")
	if len(args) == 0 {
		return nil
	}

	// Check if this is docker-proxy
	if !strings.HasSuffix(args[0], "docker-proxy") {
		return nil
	}

	target := &dockerProxyTarget{}
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "-container-ip":
			target.containerIP = args[i+1]
		case "-container-port":
			port, err := strconv.ParseUint(args[i+1], 10, 16)
			if err != nil {
				return nil
			}
			target.containerPort = uint16(port)
		}
	}

	if target.containerIP == "" || target.containerPort == 0 {
		return nil
	}

	return target
}

// isDockerProxiedConnection checks if a connection is internal docker-proxy forwarding traffic.
// Returns true if the connection should be filtered out.
func isDockerProxiedConnection(conn *model.Connection, proxies map[string]*dockerProxyTarget) bool {
	// Check if local address matches a proxy target
	laddrKey := fmt.Sprintf("%s:%d", conn.Laddr.IP, conn.Laddr.Port)
	if proxy, ok := proxies[laddrKey]; ok {
		// If the other end is the proxy IP, this is internal proxy traffic
		if proxy.proxyIP != "" && conn.Raddr.IP == proxy.proxyIP {
			return true
		}
	}

	// Check if remote address matches a proxy target
	raddrKey := fmt.Sprintf("%s:%d", conn.Raddr.IP, conn.Raddr.Port)
	if proxy, ok := proxies[raddrKey]; ok {
		// If the other end is the proxy IP, this is internal proxy traffic
		if proxy.proxyIP != "" && conn.Laddr.IP == proxy.proxyIP {
			return true
		}
	}

	return false
}

// connKey represents a connection tuple for conntrack lookup
type connKey struct {
	srcIP   string
	srcPort uint16
	dstIP   string
	dstPort uint16
}

func makeConnKey(srcIP string, srcPort uint16, dstIP string, dstPort uint16) connKey {
	return connKey{
		srcIP:   srcIP,
		srcPort: srcPort,
		dstIP:   dstIP,
		dstPort: dstPort,
	}
}

// dumpConntrackTable creates a temporary conntrack snapshot and returns a lookup map
func (s *discovery) dumpConntrackTable(ctx context.Context) (map[connKey]*network.IPTranslation, error) {
	if !s.config.EnableConntrack {
		return nil, nil
	}

	// Create config for the netlink consumer
	cfg := netconfig.New()
	cfg.ProcRoot = "/proc"

	// Respect the system-wide conntrack all namespaces setting
	// This is critical for Kubernetes environments where connections
	// originate in pod namespaces but we need to match them with
	// conntrack entries from the same namespace
	sysCfg := ddconfig.SystemProbe()
	cfg.EnableConntrackAllNamespaces = sysCfg.GetBool("system_probe_config.enable_conntrack_all_namespaces")
	if !sysCfg.IsSet("system_probe_config.enable_conntrack_all_namespaces") {
		// Default to true for discovery module to support Kubernetes pod connections
		cfg.EnableConntrackAllNamespaces = true
	}

	log.Debugf("Conntrack consumer: enable_conntrack_all_namespaces=%v", cfg.EnableConntrackAllNamespaces)

	// Create a consumer for dumping the conntrack table
	consumer, err := netlink.NewConsumer(cfg, nil)
	if err != nil {
		if errors.Is(err, netlink.ErrNotPermitted) {
			log.Warnf("Cannot dump conntrack table: insufficient permissions (need CAP_NET_ADMIN)")
		} else {
			log.Warnf("Failed to create netlink consumer: %v", err)
		}
		return nil, err
	}
	defer consumer.Stop()

	// Dump IPv4 table
	events, err := consumer.DumpTable(unix.AF_INET)
	if err != nil {
		log.Warnf("Failed to dump conntrack table: %v", err)
		return nil, err
	}

	// Build lookup map
	decoder := netlink.NewDecoder()
	translations := make(map[connKey]*network.IPTranslation)

	// Process events with timeout
	timeoutTimer := time.NewTimer(5 * time.Second)
	defer timeoutTimer.Stop()

processLoop:
	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Channel closed, dump complete
				break processLoop
			}

			// Decode the event
			conns := decoder.DecodeAndReleaseEvent(event)
			for _, conn := range conns {
				// Skip non-NAT connections
				if !netlink.IsNAT(conn) {
					continue
				}

				// Build key from original tuple
				// Origin is the original source/dest, Reply is the translated dest/source
				key := makeConnKey(
					conn.Origin.Src.Addr().String(),
					conn.Origin.Src.Port(),
					conn.Origin.Dst.Addr().String(),
					conn.Origin.Dst.Port(),
				)

				// Create IPTranslation from the Reply tuple
				// For NAT: Origin -> Reply translation
				// Reply.Dst is the translated source (what the source becomes)
				// Reply.Src is the translated destination (what the destination becomes)
				translations[key] = &network.IPTranslation{
					ReplSrcIP:   util.Address{Addr: conn.Reply.Dst.Addr()},
					ReplSrcPort: conn.Reply.Dst.Port(),
					ReplDstIP:   util.Address{Addr: conn.Reply.Src.Addr()},
					ReplDstPort: conn.Reply.Src.Port(),
				}
			}

		case <-ctx.Done():
			log.Warnf("Conntrack dump cancelled: %v", ctx.Err())
			return translations, ctx.Err()

		case <-timeoutTimer.C:
			log.Warnf("Conntrack dump timed out after 5 seconds")
			return translations, errors.New("timeout")
		}
	}

	log.Debugf("Built conntrack translation map with %d entries", len(translations))

	// Log the full translation map at trace level for debugging
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("Conntrack translation map (%d entries):", len(translations))
		for key, trans := range translations {
			log.Tracef("  %s:%d -> %s:%d  =>  %s:%d -> %s:%d",
				key.srcIP, key.srcPort,
				key.dstIP, key.dstPort,
				trans.ReplSrcIP.String(), trans.ReplSrcPort,
				trans.ReplDstIP.String(), trans.ReplDstPort)
		}
	}

	return translations, nil
}

// translateConnectionWithMap applies NAT translation from the conntrack lookup map
func translateConnectionWithMap(conn *model.Connection, translations map[connKey]*network.IPTranslation) {
	if len(translations) == 0 {
		return
	}

	// Build lookup key from connection
	key := makeConnKey(
		conn.Laddr.IP,
		conn.Laddr.Port,
		conn.Raddr.IP,
		conn.Raddr.Port,
	)

	// Look up translation
	translation, found := translations[key]
	if !found {
		log.Tracef("No NAT translation found for connection %s:%d -> %s:%d (netns=%d)",
			conn.Laddr.IP, conn.Laddr.Port,
			conn.Raddr.IP, conn.Raddr.Port,
			conn.NetNS)
		return // no NAT translation for this connection
	}

	// Apply translation to connection
	conn.TranslatedLaddr = &model.Address{
		IP:   translation.ReplSrcIP.String(),
		Port: translation.ReplSrcPort,
	}
	conn.TranslatedRaddr = &model.Address{
		IP:   translation.ReplDstIP.String(),
		Port: translation.ReplDstPort,
	}

	log.Tracef("NAT translation applied (PID=%d, netns=%d): %s:%d -> %s:%d  =>  %s:%d -> %s:%d",
		conn.PID, conn.NetNS,
		conn.Laddr.IP, conn.Laddr.Port,
		conn.Raddr.IP, conn.Raddr.Port,
		conn.TranslatedLaddr.IP, conn.TranslatedLaddr.Port,
		conn.TranslatedRaddr.IP, conn.TranslatedRaddr.Port)
}

// getConnections collects all established TCP connections from all processes.
func (s *discovery) getConnections() (*model.ConnectionsResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	// Get the list of listening ports to determine direction
	pctx := newParsingContext()
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

	// Track docker-proxy processes and their targets
	// Key is "containerIP:containerPort"
	dockerProxies := make(map[string]*dockerProxyTarget)
	dockerProxyPIDs := make(map[int]struct{})

	// First pass: detect docker-proxy processes
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue
		}
		if target := detectDockerProxy(pid); target != nil {
			key := fmt.Sprintf("%s:%d", target.containerIP, target.containerPort)
			dockerProxies[key] = target
			dockerProxyPIDs[pid] = struct{}{}
			log.Debugf("detected docker-proxy pid=%d target=%s:%d", pid, target.containerIP, target.containerPort)
		}
	}

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue // not a PID directory
		}

		// Skip docker-proxy processes - we don't want their connections
		if _, isProxy := dockerProxyPIDs[pid]; isProxy {
			continue
		}

		// Get network namespace info (for listening ports)
		ns, err := netns.GetNetNsInoFromPid(pctx.procRoot, pid)
		if err != nil {
			continue
		}

		// Cache listening ports per namespace
		if _, ok := pctx.netNsInfo[ns]; !ok {
			nsInfo, err := getNsInfo(pid)
			if err != nil {
				continue
			}
			pctx.netNsInfo[ns] = nsInfo
			// Add listening ports to the global set
			for _, info := range nsInfo.tcpSockets {
				listeningPorts[info.port] = struct{}{}
			}
		}

		// Cache established connections per namespace (similar to getNsInfo caching above).
		// /proc/<pid>/net/tcp{,6} is shared across all processes in the same network namespace,
		// so we only need to read it once per namespace.
		establishedInfo, ok := pctx.establishedConnsInfo[ns]
		if !ok {
			v4Conns, err := getEstablishedConnections(pid, "v4")
			if err != nil {
				log.Debugf("couldn't get established connections for pid %d family v4: %v", pid, err)
				v4Conns = nil
			}
			v6Conns, err := getEstablishedConnections(pid, "v6")
			if err != nil {
				// IPv6 may be disabled, not an error
				if !errors.Is(err, os.ErrNotExist) {
					log.Debugf("couldn't get established connections for pid %d family v6: %v", pid, err)
				}
				v6Conns = nil
			}
			establishedInfo = &establishedConnsNsInfo{v4: v4Conns, v6: v6Conns}
			pctx.establishedConnsInfo[ns] = establishedInfo
		}

		// Get the socket inodes owned by this process
		openFileInfo, err := getOpenFilesInfo(int32(pid), pctx.readlinkBuffer)
		if err != nil {
			continue
		}

		// Use cached data to match socket inodes to established connections
		for _, family := range []string{"v4", "v6"} {
			var establishedConns map[uint64]*establishedConnInfo
			if family == "v4" {
				establishedConns = establishedInfo.v4
			} else {
				establishedConns = establishedInfo.v6
			}
			if establishedConns == nil {
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

	// Discover proxy IPs from connections and filter proxied connections
	if len(dockerProxies) > 0 {
		// Discover proxy IPs: for each connection, if one end matches a proxy target,
		// the other end is the proxy IP
		for i := range connections {
			conn := &connections[i]
			laddrKey := fmt.Sprintf("%s:%d", conn.Laddr.IP, conn.Laddr.Port)
			if proxy, ok := dockerProxies[laddrKey]; ok && proxy.proxyIP == "" {
				proxy.proxyIP = conn.Raddr.IP
				log.Debugf("discovered docker-proxy IP %s for target %s", proxy.proxyIP, laddrKey)
			}
			raddrKey := fmt.Sprintf("%s:%d", conn.Raddr.IP, conn.Raddr.Port)
			if proxy, ok := dockerProxies[raddrKey]; ok && proxy.proxyIP == "" {
				proxy.proxyIP = conn.Laddr.IP
				log.Debugf("discovered docker-proxy IP %s for target %s", proxy.proxyIP, raddrKey)
			}
		}

		// Filter out proxied connections
		filtered := make([]model.Connection, 0, len(connections))
		for _, conn := range connections {
			if isDockerProxiedConnection(&conn, dockerProxies) {
				log.Debugf("filtering docker-proxied connection %s:%d -> %s:%d",
					conn.Laddr.IP, conn.Laddr.Port, conn.Raddr.IP, conn.Raddr.Port)
				continue
			}
			filtered = append(filtered, conn)
		}
		connections = filtered
	}

	// Collect Cilium connections if enabled
	var ciliumConnections []model.Connection
	if s.config.EnableCiliumConntracker {
		if s.ciliumConntracker == nil {
			var err error
			s.ciliumConntracker, err = newCiliumConntracker()
			if err != nil {
				log.Warnf("Failed to initialize Cilium conntracker: %v", err)
			}
		}
		if s.ciliumConntracker != nil {
			ciliumConns, err := s.ciliumConntracker.getConnections()
			if err != nil {
				log.Warnf("Failed to get Cilium connections: %v", err)
			} else {
				ciliumConnections = ciliumConns
				log.Debugf("Retrieved %d connections from Cilium", len(ciliumConnections))
			}
		}
	}

	// Merge connections (prefer Cilium NAT if both sources have same connection)
	connections = mergeConnections(connections, ciliumConnections)

	// Apply NAT translations using conntrack
	if s.config.EnableConntrack {
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		translations, err := s.dumpConntrackTable(ctxTimeout)
		if err != nil {
			log.Warnf("Failed to dump conntrack table, continuing without NAT resolution: %v", err)
		} else if translations != nil {
			translationsFound := 0
			for i := range connections {
				translateConnectionWithMap(&connections[i], translations)
				if connections[i].TranslatedLaddr != nil || connections[i].TranslatedRaddr != nil {
					translationsFound++
				}
			}

			if translationsFound > 0 {
				log.Debugf("Found NAT translations for %d/%d connections", translationsFound, len(connections))
			}
		}
	}

	return &model.ConnectionsResponse{
		Connections: connections,
	}, nil
}

// mergeConnections enriches /proc connections with NAT information from Cilium.
// Only connections that exist in /proc are kept (we need PID information).
// Cilium-only connections are skipped since they lack PID info.
func mergeConnections(procConns, ciliumConns []model.Connection) []model.Connection {
	if len(ciliumConns) == 0 {
		return procConns
	}

	// Map Cilium connections by 4-tuple key for quick lookup
	type connKey struct {
		localIP    string
		localPort  uint16
		remoteIP   string
		remotePort uint16
	}

	ciliumNATMap := make(map[connKey]*model.Address)

	// Build lookup map of Cilium NAT translations
	for i := range ciliumConns {
		if ciliumConns[i].TranslatedRaddr != nil {
			key := connKey{
				localIP:    ciliumConns[i].Laddr.IP,
				localPort:  ciliumConns[i].Laddr.Port,
				remoteIP:   ciliumConns[i].Raddr.IP,
				remotePort: ciliumConns[i].Raddr.Port,
			}
			ciliumNATMap[key] = ciliumConns[i].TranslatedRaddr
		}
	}

	// Enrich /proc connections with Cilium NAT info
	enrichedCount := 0
	for i := range procConns {
		key := connKey{
			localIP:    procConns[i].Laddr.IP,
			localPort:  procConns[i].Laddr.Port,
			remoteIP:   procConns[i].Raddr.IP,
			remotePort: procConns[i].Raddr.Port,
		}

		if translatedAddr, ok := ciliumNATMap[key]; ok {
			procConns[i].TranslatedRaddr = translatedAddr
			enrichedCount++
			log.Debugf("Enriched connection with Cilium NAT: %s:%d -> %s:%d (translated to %s:%d)",
				procConns[i].Laddr.IP, procConns[i].Laddr.Port,
				procConns[i].Raddr.IP, procConns[i].Raddr.Port,
				translatedAddr.IP, translatedAddr.Port)
		}
	}

	if enrichedCount > 0 {
		log.Debugf("Enriched %d connections with Cilium NAT information", enrichedCount)
	}

	return procConns
}
