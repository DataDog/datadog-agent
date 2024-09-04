// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathServices = "/services"
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// serviceInfo holds process data that should be cached between calls to the
// endpoint.
type serviceInfo struct {
	name               string
	nameFromDDService  bool
	language           language.Language
	apmInstrumentation apm.Instrumentation
	cmdLine            []string
	startTimeSecs      uint64
}

// discovery is an implementation of the Module interface for the discovery module.
type discovery struct {
	mux *sync.RWMutex
	// cache maps pids to data that should be cached between calls to the endpoint.
	cache map[int32]*serviceInfo

	// privilegedDetector is used to detect the language of a process.
	privilegedDetector privileged.LanguageDetector

	// scrubber is used to remove potentially sensitive data from the command line
	scrubber *procutil.DataScrubber
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(*sysconfigtypes.Config, module.FactoryDependencies) (module.Module, error) {
	return &discovery{
		mux:                &sync.RWMutex{},
		cache:              make(map[int32]*serviceInfo),
		privilegedDetector: privileged.NewLanguageDetector(),
		scrubber:           procutil.NewDefaultDataScrubber(),
	}, nil
}

// GetStats returns the stats of the discovery module.
func (s *discovery) GetStats() map[string]interface{} {
	return nil
}

// Register registers the discovery module with the provided HTTP mux.
func (s *discovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/status", s.handleStatusEndpoint)
	httpMux.HandleFunc(pathServices, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))
	return nil
}

// Close cleans resources used by the discovery module.
func (s *discovery) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()
	clear(s.cache)
}

// handleStatusEndpoint is the handler for the /status endpoint.
// Reports the status of the discovery module.
func (s *discovery) handleStatusEndpoint(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("Discovery Module is running"))
}

// handleServers is the handler for the /services endpoint.
// Returns the list of currently running services.
func (s *discovery) handleServices(w http.ResponseWriter, _ *http.Request) {
	services, err := s.getServices()
	if err != nil {
		_ = log.Errorf("failed to handle /discovery%s: %v", pathServices, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := &model.ServicesResponse{
		Services: *services,
	}
	utils.WriteAsJSON(w, resp)
}

const prefix = "socket:["

// getSockets get a list of socket inode numbers opened by a process
func getSockets(pid int32) ([]uint64, error) {
	statPath := kernel.HostProc(fmt.Sprintf("%d/fd", pid))
	d, err := os.Open(statPath)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	fnames, err := d.Readdirnames(-1)

	if err != nil {
		return nil, err
	}
	var sockets []uint64
	for _, fd := range fnames {
		fullPath, err := os.Readlink(filepath.Join(statPath, fd))
		if err != nil {
			continue
		}
		if strings.HasPrefix(fullPath, prefix) {
			sock, err := strconv.ParseUint(fullPath[len(prefix):len(fullPath)-1], 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
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
func newNetIPSocket(file string, expectedState uint64) (map[uint64]uint16, error) {
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
	tcp, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/tcp", pid)), tcpListen)
	if err != nil {
		log.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	udp, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/udp", pid)), udpListen)
	if err != nil {
		log.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	tcpv6, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/tcp6", pid)), tcpListen)
	if err != nil {
		log.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	udpv6, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/udp6", pid)), udpListen)
	if err != nil {
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
	procRoot  string
	netNsInfo map[uint32]*namespaceInfo
}

// getServiceInfo gets the service information for a process using the
// servicedetector module.
func (s *discovery) getServiceInfo(proc *process.Process) (*serviceInfo, error) {
	cmdline, err := proc.CmdlineSlice()
	if err != nil {
		return nil, err
	}

	envs, err := getEnvs(proc)
	if err != nil {
		return nil, err
	}

	exe, err := proc.Exe()
	if err != nil {
		return nil, err
	}

	createTime, err := proc.CreateTime()
	if err != nil {
		return nil, err
	}

	contextMap := make(usm.DetectorContextMap)

	root := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "root")
	name, fromDDService := servicediscovery.GetServiceName(cmdline, envs, root, contextMap)
	lang := language.FindInArgs(exe, cmdline)
	if lang == "" {
		lang = language.FindUsingPrivilegedDetector(s.privilegedDetector, proc.Pid)
	}
	apmInstrumentation := apm.Detect(int(proc.Pid), cmdline, envs, lang, contextMap)

	return &serviceInfo{
		name:               name,
		language:           lang,
		apmInstrumentation: apmInstrumentation,
		nameFromDDService:  fromDDService,
		cmdLine:            sanitizeCmdLine(s.scrubber, cmdline),
		startTimeSecs:      uint64(createTime / 1000),
	}, nil
}

// customNewProcess is the same implementation as process.NewProcess but without calling CreateTimeWithContext, which
// is not needed and costly for the discovery module.
func customNewProcess(pid int32) (*process.Process, error) {
	p := &process.Process{
		Pid: pid,
	}

	exists, err := process.PidExists(pid)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, process.ErrorProcessNotRunning
	}
	return p, nil
}

// ignoreComms is a list of process names (matched against /proc/PID/comm) to
// never report as a service. Note that comm is limited to 16 characters.
var ignoreComms = map[string]struct{}{
	"sshd":             {},
	"dhclient":         {},
	"systemd":          {},
	"systemd-resolved": {},
	"systemd-networkd": {},
	"datadog-agent":    {},
	"livenessprobe":    {},
	"docker-proxy":     {},
}

// getService gets information for a single service.
func (s *discovery) getService(context parsingContext, pid int32) *model.Service {
	proc, err := customNewProcess(pid)
	if err != nil {
		return nil
	}

	comm, err := proc.Name()
	if err != nil {
		return nil
	}

	if _, found := ignoreComms[comm]; found {
		return nil
	}

	sockets, err := getSockets(pid)
	if err != nil {
		return nil
	}
	if len(sockets) == 0 {
		return nil
	}

	ns, err := kernel.GetNetNsInoFromPid(context.procRoot, int(pid))
	if err != nil {
		return nil
	}

	// The socket and network address information are different for each
	// network namespace.  Since namespaces can be shared between multiple
	// processes, we cache them to only parse them once per call to this
	// function.
	nsInfo, ok := context.netNsInfo[ns]
	if !ok {
		nsInfo, err = getNsInfo(int(pid))
		if err != nil {
			return nil
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
		return nil
	}

	rss, err := getRSS(proc)
	if err != nil {
		return nil
	}

	var info *serviceInfo
	s.mux.RLock()
	cached, ok := s.cache[pid]
	s.mux.RUnlock()
	if ok {
		info = cached
	} else {
		info, err = s.getServiceInfo(proc)
		if err != nil {
			return nil
		}

		s.mux.Lock()
		s.cache[pid] = info
		s.mux.Unlock()
	}

	nameSource := "generated"
	if info.nameFromDDService {
		nameSource = "provided"
	}

	return &model.Service{
		PID:                int(pid),
		Name:               info.name,
		NameSource:         nameSource,
		Ports:              ports,
		APMInstrumentation: string(info.apmInstrumentation),
		Language:           string(info.language),
		RSS:                rss,
		CommandLine:        info.cmdLine,
		StartTimeSecs:      info.startTimeSecs,
	}
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it.
func (s *discovery) cleanCache(alivePids map[int32]struct{}) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for pid := range s.cache {
		if _, alive := alivePids[pid]; alive {
			continue
		}

		delete(s.cache, pid)
	}
}

// getStatus returns the list of currently running services.
func (s *discovery) getServices() (*[]model.Service, error) {
	procRoot := kernel.ProcFSRoot()
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	context := parsingContext{
		procRoot:  procRoot,
		netNsInfo: make(map[uint32]*namespaceInfo),
	}

	var services []model.Service
	alivePids := make(map[int32]struct{}, len(pids))

	for _, pid := range pids {
		alivePids[pid] = struct{}{}

		service := s.getService(context, pid)
		if service == nil {
			continue
		}

		services = append(services, *service)
	}

	s.cleanCache(alivePids)

	return &services, nil
}
