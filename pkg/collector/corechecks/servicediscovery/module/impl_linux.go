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
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/detector"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	pathServices = "/services"

	// Use a low cache validity to ensure that we refresh information every time
	// the check is run if needed. This is the same as cacheValidityNoRT in
	// pkg/process/checks/container.go.
	containerCacheValidity = 2 * time.Second

	// The maximum number of times that we check if a process has open ports
	// before ignoring it forever.
	maxPortCheckTries = 10
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// serviceInfo holds process data that should be cached between calls to the
// endpoint.
type serviceInfo struct {
	name                       string
	generatedName              string
	generatedNameSource        string
	additionalGeneratedNames   []string
	containerServiceName       string
	containerServiceNameSource string
	ddServiceName              string
	ddServiceInjected          bool
	ports                      []uint16
	checkedContainerData       bool
	language                   language.Language
	apmInstrumentation         apm.Instrumentation
	cmdLine                    []string
	startTimeMilli             uint64
	rss                        uint64
	cpuTime                    uint64
	cpuUsage                   float64
	containerID                string
	lastHeartbeat              int64
	addedToMap                 bool
	rxBytes                    uint64
	txBytes                    uint64
	rxBps                      float64
	txBps                      float64
}

// toModelService fills the model.Service struct pointed to by out, using the
// service info to do it.
func (i *serviceInfo) toModelService(pid int32, out *model.Service) *model.Service {
	if i == nil {
		log.Warn("toModelService called with nil pointer")
		return nil
	}

	out.PID = int(pid)
	out.Name = i.name
	out.GeneratedName = i.generatedName
	out.GeneratedNameSource = i.generatedNameSource
	out.AdditionalGeneratedNames = i.additionalGeneratedNames
	out.ContainerServiceName = i.containerServiceName
	out.ContainerServiceNameSource = i.containerServiceNameSource
	out.DDService = i.ddServiceName
	out.DDServiceInjected = i.ddServiceInjected
	out.Ports = i.ports
	out.APMInstrumentation = string(i.apmInstrumentation)
	out.Language = string(i.language)
	out.Type = string(servicetype.Detect(i.ports))
	out.RSS = i.rss
	out.CommandLine = i.cmdLine
	out.StartTimeMilli = i.startTimeMilli
	out.CPUCores = i.cpuUsage
	out.ContainerID = i.containerID
	out.LastHeartbeat = i.lastHeartbeat
	out.RxBytes = i.rxBytes
	out.TxBytes = i.txBytes
	out.RxBps = i.rxBps
	out.TxBps = i.txBps

	return out
}

type timeProvider interface {
	Now() time.Time
}

type realTime struct{}

func (realTime) Now() time.Time { return time.Now() }

type pidSet map[int32]struct{}

func (s pidSet) has(pid int32) bool {
	_, present := s[pid]
	return present
}

func (s pidSet) add(pid int32) {
	s[pid] = struct{}{}
}

func (s pidSet) remove(pid int32) {
	delete(s, pid)
}

// discovery is an implementation of the Module interface for the discovery module.
type discovery struct {
	config *discoveryConfig

	mux *sync.RWMutex

	// cache maps pids to data that should be cached between calls to the endpoint.
	cache map[int32]*serviceInfo

	// noPortTries stores the number of times in a row that we did not find
	// open ports for this process.
	noPortTries map[int32]int

	// potentialServices stores processes that we have seen once in the previous
	// iteration, but not yet confirmed to be a running service.
	potentialServices pidSet

	// runningServices stores services that we have previously confirmed as
	// running.
	runningServices pidSet

	// ignorePids stores processes to be excluded from discovery
	ignorePids pidSet

	// privilegedDetector is used to detect the language of a process.
	privilegedDetector privileged.LanguageDetector

	// scrubber is used to remove potentially sensitive data from the command line
	scrubber *procutil.DataScrubber

	// lastGlobalCPUTime stores the total cpu time of the system from the last time
	// the endpoint was called.
	lastGlobalCPUTime uint64

	// lastCPUTimeUpdate is the last time lastGlobalCPUTime was updated.
	lastCPUTimeUpdate time.Time

	lastNetworkStatsUpdate time.Time

	wmeta  workloadmeta.Component
	tagger tagger.Component

	timeProvider timeProvider
	network      networkCollector
}

type networkCollectorFactory func(cfg *discoveryConfig) (networkCollector, error)

func newDiscoveryWithNetwork(wmeta workloadmeta.Component, tagger tagger.Component, tp timeProvider, getNetworkCollector networkCollectorFactory) *discovery {
	cfg := newConfig()

	var network networkCollector
	if cfg.networkStatsEnabled {
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
		config:             cfg,
		mux:                &sync.RWMutex{},
		cache:              make(map[int32]*serviceInfo),
		noPortTries:        make(map[int32]int),
		potentialServices:  make(pidSet),
		runningServices:    make(pidSet),
		ignorePids:         make(pidSet),
		privilegedDetector: privileged.NewLanguageDetector(),
		scrubber:           procutil.NewDefaultDataScrubber(),
		wmeta:              wmeta,
		tagger:             tagger,
		timeProvider:       tp,
		network:            network,
	}
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	d := newDiscoveryWithNetwork(deps.WMeta, deps.Tagger, realTime{}, newNetworkCollector)

	return d, nil
}

// GetStats returns the stats of the discovery module.
func (s *discovery) GetStats() map[string]interface{} {
	return nil
}

// Register registers the discovery module with the provided HTTP mux.
func (s *discovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/status", s.handleStatusEndpoint)
	httpMux.HandleFunc("/state", s.handleStateEndpoint)
	httpMux.HandleFunc("/debug", s.handleDebugEndpoint)
	httpMux.HandleFunc(pathServices, utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, s.handleServices))
	return nil
}

// Close cleans resources used by the discovery module.
func (s *discovery) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.cleanCache(pidSet{})
	if s.network != nil {
		s.network.close()
	}
	clear(s.cache)
	clear(s.noPortTries)
	clear(s.ignorePids)
}

// handleStatusEndpoint is the handler for the /status endpoint.
// Reports the status of the discovery module.
func (s *discovery) handleStatusEndpoint(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("Discovery Module is running"))
}

type state struct {
	Cache                  map[int]*model.Service `json:"cache"`
	NoPortTries            map[int]int            `json:"no_port_tries"`
	PotentialServices      []int                  `json:"potential_services"`
	RunningServices        []int                  `json:"running_services"`
	IgnorePids             []int                  `json:"ignore_pids"`
	LastGlobalCPUTime      uint64                 `json:"last_global_cpu_time"`
	LastCPUTimeUpdate      int64                  `json:"last_cpu_time_update"`
	LastNetworkStatsUpdate int64                  `json:"last_network_stats_update"`
	NetworkEnabled         bool                   `json:"network_enabled"`
}

// handleStateEndpoint is the handler for the /state endpoint.
// Returns the internal state of the discovery module.
func (s *discovery) handleStateEndpoint(w http.ResponseWriter, _ *http.Request) {
	s.mux.Lock()
	defer s.mux.Unlock()

	state := &state{
		Cache:             make(map[int]*model.Service, len(s.cache)),
		NoPortTries:       make(map[int]int, len(s.noPortTries)),
		PotentialServices: make([]int, 0, len(s.potentialServices)),
		RunningServices:   make([]int, 0, len(s.runningServices)),
		IgnorePids:        make([]int, 0, len(s.ignorePids)),
		NetworkEnabled:    s.network != nil,
	}

	for pid, info := range s.cache {
		service := &model.Service{}
		info.toModelService(pid, service)
		state.Cache[int(pid)] = service
	}

	for pid, tries := range s.noPortTries {
		state.NoPortTries[int(pid)] = tries
	}

	for pid := range s.potentialServices {
		state.PotentialServices = append(state.PotentialServices, int(pid))
	}

	for pid := range s.runningServices {
		state.RunningServices = append(state.RunningServices, int(pid))
	}

	for pid := range s.ignorePids {
		state.IgnorePids = append(state.IgnorePids, int(pid))
	}

	state.LastGlobalCPUTime = s.lastGlobalCPUTime
	state.LastCPUTimeUpdate = s.lastCPUTimeUpdate.Unix()
	state.LastNetworkStatsUpdate = s.lastNetworkStatsUpdate.Unix()

	utils.WriteAsJSON(w, state)
}

func (s *discovery) handleDebugEndpoint(w http.ResponseWriter, _ *http.Request) {
	s.mux.Lock()
	defer s.mux.Unlock()

	services := make([]model.Service, 0)

	procRoot := kernel.ProcFSRoot()
	pids, err := process.Pids()
	if err != nil {
		utils.WriteAsJSON(w, "could not get PIDs")
		return
	}

	context := parsingContext{
		procRoot:  procRoot,
		netNsInfo: make(map[uint32]*namespaceInfo),
	}

	containers := s.getContainersMap()
	containerTagsCache := make(map[string][]string)
	for _, pid := range pids {
		service := s.getService(context, pid)
		if service == nil {
			continue
		}
		s.enrichContainerData(service, containers, containerTagsCache)

		services = append(services, *service)
	}

	utils.WriteAsJSON(w, services)
}

// handleServers is the handler for the /services endpoint.
// Returns the list of currently running services.
func (s *discovery) handleServices(w http.ResponseWriter, req *http.Request) {
	params, err := parseParams(req.URL.Query())
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

	utils.WriteAsJSON(w, services)
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
	if err != nil {
		log.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	udpv6, err := newNetIPSocket(kernel.HostProc(fmt.Sprintf("%d/net/udp6", pid)), udpListen,
		func(port uint16) bool {
			return network.IsPortInEphemeralRange(network.AFINET6, network.UDP, port) == network.EphemeralTrue
		})
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

// addIgnoredPid store excluded pid.
func (s *discovery) addIgnoredPid(pid int32) {
	s.ignorePids[pid] = struct{}{}
}

// shouldIgnorePid returns true if process should be excluded from handling.
func (s *discovery) shouldIgnorePid(pid int32) bool {
	_, found := s.ignorePids[pid]
	return found
}

// shouldIgnoreService returns true if the service should be excluded from handling.
func (s *discovery) shouldIgnoreService(name string) bool {
	if len(s.config.ignoreServices) == 0 {
		return false
	}
	_, found := s.config.ignoreServices[name]

	return found
}

// getServiceInfo gets the service information for a process using the
// servicedetector module.
func (s *discovery) getServiceInfo(pid int32) (*serviceInfo, error) {
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

	createTime, err := proc.CreateTime()
	if err != nil {
		return nil, err
	}

	root := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "root")
	lang := language.FindInArgs(exe, cmdline)
	if lang == "" {
		lang = language.FindUsingPrivilegedDetector(s.privilegedDetector, proc.Pid)
	}
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
	apmInstrumentation := apm.Detect(lang, ctx)

	name := nameMeta.DDService
	if name == "" {
		name = nameMeta.Name
	}

	cmdline, _ = s.scrubber.ScrubCommand(cmdline)

	return &serviceInfo{
		name:                     name,
		generatedName:            nameMeta.Name,
		generatedNameSource:      string(nameMeta.Source),
		additionalGeneratedNames: nameMeta.AdditionalNames,
		ddServiceName:            nameMeta.DDService,
		language:                 lang,
		apmInstrumentation:       apmInstrumentation,
		ddServiceInjected:        nameMeta.DDServiceInjected,
		cmdLine:                  truncateCmdline(lang, cmdline),
		startTimeMilli:           uint64(createTime),
	}, nil
}

// maxNumberOfPorts is the maximum number of listening ports which we report per
// service.
const maxNumberOfPorts = 50

func (s *discovery) getPorts(context parsingContext, pid int32) ([]uint16, error) {
	sockets, err := getSockets(pid)
	if err != nil {
		return nil, err
	}
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

// getService gets information for a single service.
func (s *discovery) getService(context parsingContext, pid int32) *model.Service {
	if s.shouldIgnorePid(pid) {
		return nil
	}
	if s.shouldIgnoreComm(pid) {
		s.addIgnoredPid(pid)
		return nil
	}

	ports, err := s.getPorts(context, pid)
	if err != nil {
		return nil
	}
	if len(ports) == 0 {
		tries := s.noPortTries[pid]
		tries++
		s.noPortTries[pid] = tries

		if tries >= maxPortCheckTries {
			log.Tracef("[pid: %d] ignoring due to no ports", pid)
			s.addIgnoredPid(pid)
			delete(s.noPortTries, pid)
		}
		return nil
	}

	// Reset the try counter since we only count tries in a row.
	delete(s.noPortTries, pid)

	rss, err := getRSS(pid)
	if err != nil {
		return nil
	}

	var info *serviceInfo
	cached, ok := s.cache[pid]
	if ok {
		info = cached
	} else {
		info, err = s.getServiceInfo(pid)
		if err != nil {
			return nil
		}

		s.cache[pid] = info
	}

	if s.shouldIgnoreService(info.name) {
		s.addIgnoredPid(pid)
		return nil
	}

	service := &model.Service{}
	info.toModelService(pid, service)
	service.Ports = ports
	service.RSS = rss

	return service
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it. This function is not thread-safe and it is up to the caller to ensure
// s.mux is locked.
func (s *discovery) cleanCache(alivePids pidSet) {
	for pid, info := range s.cache {
		if alivePids.has(pid) {
			continue
		}

		if info.addedToMap {
			err := s.network.removePid(uint32(pid))
			if err != nil {
				log.Warn("unable to remove pid from network collector", pid, err)
			}
		}

		delete(s.cache, pid)
	}

	for pid := range s.noPortTries {
		if alivePids.has(pid) {
			continue
		}

		delete(s.noPortTries, pid)
	}
}

func (s *discovery) updateNetworkStats(deltaSeconds float64, response *model.ServicesResponse) {
	for pid, info := range s.cache {
		if !info.addedToMap {
			err := s.network.addPid(uint32(pid))
			if err == nil {
				info.addedToMap = true
			} else {
				log.Warnf("unable to add to network collector %v: %v", pid, err)
			}
			continue
		}

		stats, err := s.network.getStats(uint32(pid))
		if err != nil {
			log.Warnf("unable to get network stats %v: %v", pid, err)
			continue
		}

		deltaRx := stats.Rx - info.rxBytes
		deltaTx := stats.Tx - info.txBytes

		info.rxBps = float64(deltaRx) / deltaSeconds
		info.txBps = float64(deltaTx) / deltaSeconds

		info.rxBytes = stats.Rx
		info.txBytes = stats.Tx
	}

	updateResponseNetworkStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := s.cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.RxBps = info.rxBps
			service.TxBps = info.txBps
			service.RxBytes = info.rxBytes
			service.TxBytes = info.txBytes
		}
	}

	updateResponseNetworkStats(response.StartedServices)
	updateResponseNetworkStats(response.HeartbeatServices)
}

func (s *discovery) maybeUpdateNetworkStats(response *model.ServicesResponse) {
	if s.network == nil {
		return
	}

	now := s.timeProvider.Now()
	delta := now.Sub(s.lastNetworkStatsUpdate)
	if delta < s.config.networkStatsPeriod {
		return
	}

	deltaSeconds := delta.Seconds()

	s.updateNetworkStats(deltaSeconds, response)

	s.lastNetworkStatsUpdate = now
}

// cleanPidSets deletes dead PIDs from the provided pidSets. This function is not
// thread-safe and it is up to the caller to ensure s.mux is locked.
func (s *discovery) cleanPidSets(alivePids pidSet, sets ...pidSet) {
	for _, set := range sets {
		for pid := range set {
			if alivePids.has(pid) {
				continue
			}

			delete(set, pid)
		}
	}
}

// updateServicesCPUStats updates the CPU stats of cached services, as well as the
// global CPU time cache for future updates. This function is not thread-safe and
// it is up to the caller to ensure s.mux is locked.
func (s *discovery) updateServicesCPUStats(response *model.ServicesResponse) error {
	if time.Since(s.lastCPUTimeUpdate) < s.config.cpuUsageUpdateDelay {
		return nil
	}

	globalCPUTime, err := getGlobalCPUTime()
	if err != nil {
		return fmt.Errorf("could not get global CPU time: %w", err)
	}

	for pid, info := range s.cache {
		_ = updateCPUCoresStats(int(pid), info, s.lastGlobalCPUTime, globalCPUTime)
	}

	updateResponseCPUStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := s.cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.CPUCores = info.cpuUsage
		}
	}

	updateResponseCPUStats(response.StartedServices)
	updateResponseCPUStats(response.HeartbeatServices)

	s.lastGlobalCPUTime = globalCPUTime
	s.lastCPUTimeUpdate = time.Now()

	return nil
}

func getServiceNameFromContainerTags(tags []string) (string, string) {
	// The tags we look for service name generation, in their priority order.
	// The map entries will be filled as we go through the containers tags.
	tagsPriority := []struct {
		tagName  string
		tagValue *string
	}{
		{"service", nil},
		{"app", nil},
		{"short_image", nil},
		{"kube_container_name", nil},
		{"kube_deployment", nil},
		{"kube_service", nil},
	}

	// Sort the tags to make the function deterministic
	slices.Sort(tags)

	for _, tag := range tags {
		// Get index of separator between name and value
		sepIndex := strings.IndexRune(tag, ':')
		if sepIndex < 0 || sepIndex >= len(tag)-1 {
			// Malformed tag; we skip it
			continue
		}

		for i := range tagsPriority {
			if tagsPriority[i].tagValue != nil {
				// We have seen this tag before, we don't need another value.
				continue
			}

			if tag[:sepIndex] != tagsPriority[i].tagName {
				// Not a tag we care about; we skip it
				continue
			}

			value := tag[sepIndex+1:]
			tagsPriority[i].tagValue = &value
			break
		}
	}

	for _, tag := range tagsPriority {
		if tag.tagValue == nil {
			continue
		}

		log.Debugf("Using %v:%v tag for service name", tag.tagName, *tag.tagValue)
		return tag.tagName, *tag.tagValue
	}

	return "", ""
}

func (s *discovery) getContainersMap() map[int]*workloadmeta.Container {
	containers := s.wmeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	containersMap := make(map[int]*workloadmeta.Container, len(containers))

	metricsProvider := metrics.GetProvider(option.New(s.wmeta))

	for _, container := range containers {
		collector := metricsProvider.GetCollector(provider.NewRuntimeMetadata(
			string(container.Runtime),
			string(container.RuntimeFlavor),
		))
		if collector == nil {
			containersMap[int(container.PID)] = container
			continue
		}

		pids, err := collector.GetPIDs(container.Namespace, container.ID, containerCacheValidity)
		if err != nil || len(pids) == 0 {
			containersMap[int(container.PID)] = container
			continue
		}

		for _, pid := range pids {
			containersMap[int(pid)] = container
		}
	}
	return containersMap
}

func (s *discovery) getProcessContainerInfo(pid int, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) (string, []string, bool) {
	container, ok := containers[pid]
	if !ok {
		return "", nil, false
	}

	tags, ok := containerTagsCache[container.EntityID.ID]
	if ok {
		return container.EntityID.ID, tags, true
	}

	containerID := container.EntityID.ID
	collectorTags := container.CollectorTags

	// Getting the tags from the tagger. This logic is borrowed from
	// the GetContainers helper in pkg/process/util/containers.
	entityID := types.NewEntityID(types.ContainerID, containerID)
	entityTags, err := s.tagger.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Tracef("Could not get tags for container %s: %v", containerID, err)
		return containerID, collectorTags, false
	}
	tags = append(collectorTags, entityTags...)
	containerTagsCache[containerID] = tags

	return containerID, tags, true
}

func (s *discovery) enrichContainerData(service *model.Service, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) {
	containerID, containerTags, ok := s.getProcessContainerInfo(service.PID, containers, containerTagsCache)
	if !ok {
		return
	}

	service.ContainerID = containerID
	service.ContainerTags = containerTags

	// We checked the container tags before, no need to do it again.
	if service.CheckedContainerData {
		return
	}

	tagName, serviceName := getServiceNameFromContainerTags(containerTags)
	service.ContainerServiceName = serviceName
	service.ContainerServiceNameSource = tagName
	service.CheckedContainerData = true

	serviceInfo, ok := s.cache[int32(service.PID)]
	if ok {
		serviceInfo.containerServiceName = serviceName
		serviceInfo.containerServiceNameSource = tagName
		serviceInfo.checkedContainerData = true
		serviceInfo.containerID = containerID
	}
}

func (s *discovery) updateCacheInfo(response *model.ServicesResponse, now time.Time) {
	updateCachedHeartbeat := func(service *model.Service) {
		info, ok := s.cache[int32(service.PID)]
		if !ok {
			log.Warnf("could not access service info from the cache when update last heartbeat for PID %v start event", service.PID)
			return
		}

		info.lastHeartbeat = now.Unix()
		info.ports = service.Ports
		info.rss = service.RSS
	}

	for i := range response.StartedServices {
		service := &response.StartedServices[i]
		updateCachedHeartbeat(service)
	}

	for i := range response.HeartbeatServices {
		service := &response.HeartbeatServices[i]
		updateCachedHeartbeat(service)
	}
}

// handleStoppedServices verifies services previously seen and registered as
// running are still alive. If not, it will use the latest cached information
// about them to generate a stop event for the service. This function is not
// thread-safe and it is up to the caller to ensure s.mux is locked.
func (s *discovery) handleStoppedServices(response *model.ServicesResponse, alivePids pidSet) {
	for pid := range s.runningServices {
		if alivePids.has(pid) {
			continue
		}

		s.runningServices.remove(pid)
		info, ok := s.cache[pid]
		if !ok {
			log.Warnf("could not get service from the cache to generate a stopped service event for PID %v", pid)
			continue
		}

		// Build service struct in place in the slice
		response.StoppedServices = append(response.StoppedServices, model.Service{})
		info.toModelService(pid, &response.StoppedServices[len(response.StoppedServices)-1])
	}
}

// getStatus returns the list of currently running services.
func (s *discovery) getServices(params params) (*model.ServicesResponse, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	procRoot := kernel.ProcFSRoot()
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	context := parsingContext{
		procRoot:  procRoot,
		netNsInfo: make(map[uint32]*namespaceInfo),
	}

	response := &model.ServicesResponse{
		StartedServices:   make([]model.Service, 0, len(s.potentialServices)),
		StoppedServices:   make([]model.Service, 0),
		HeartbeatServices: make([]model.Service, 0),
	}

	alivePids := make(pidSet, len(pids))
	containers := s.getContainersMap()
	containerTagsCache := make(map[string][]string)

	now := s.timeProvider.Now()

	for _, pid := range pids {
		alivePids.add(pid)

		_, knownService := s.runningServices[pid]
		if knownService {
			info, ok := s.cache[pid]
			if !ok {
				// Should never happen
				continue
			}

			serviceHeartbeatTime := time.Unix(info.lastHeartbeat, 0)
			if now.Sub(serviceHeartbeatTime).Truncate(time.Minute) < params.heartbeatTime {
				// We only need to refresh the service info (ports, etc.) for
				// this service if it's time to send a heartbeat.
				continue
			}
		}

		service := s.getService(context, pid)
		if service == nil {
			continue
		}
		s.enrichContainerData(service, containers, containerTagsCache)

		if knownService {
			service.LastHeartbeat = now.Unix()
			response.HeartbeatServices = append(response.HeartbeatServices, *service)
			continue
		}

		if _, ok := s.potentialServices[pid]; ok {
			// We have seen it first in the previous call of getServices, so it
			// is confirmed to be running.
			s.runningServices.add(pid)
			delete(s.potentialServices, pid)
			service.LastHeartbeat = now.Unix()
			response.StartedServices = append(response.StartedServices, *service)
			continue
		}

		// This is a new potential service
		s.potentialServices.add(pid)
		log.Debugf("[pid: %d] adding process to potential: %s", pid, service.Name)
	}

	s.updateCacheInfo(response, now)
	s.handleStoppedServices(response, alivePids)

	s.cleanCache(alivePids)
	s.cleanPidSets(alivePids, s.ignorePids, s.potentialServices)

	if err = s.updateServicesCPUStats(response); err != nil {
		log.Warnf("updating services CPU stats: %s", err)
	}

	s.maybeUpdateNetworkStats(response)

	response.RunningServicesCount = len(s.runningServices)

	return response, nil
}
