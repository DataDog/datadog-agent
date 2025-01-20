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

	agentPayload "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathServices = "/services"

	heartbeatTime = 15 * time.Minute
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// serviceInfo holds process data that should be cached between calls to the
// endpoint.
type serviceInfo struct {
	name                       string
	generatedName              string
	generatedNameSource        string
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
}

// toModelService fills the model.Service struct pointed to by out, using the
// service info to do it.
func (i *serviceInfo) toModelService(pid int32, out *model.Service) {
	if i == nil {
		log.Warn("toModelService called with nil pointer")
		return
	}

	out.PID = int(pid)
	out.Name = i.name
	out.GeneratedName = i.generatedName
	out.GeneratedNameSource = i.generatedNameSource
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

	containerProvider proccontainers.ContainerProvider
	timeProvider      timeProvider
}

func newDiscovery(containerProvider proccontainers.ContainerProvider, tp timeProvider) *discovery {
	return &discovery{
		config:             newConfig(),
		mux:                &sync.RWMutex{},
		cache:              make(map[int32]*serviceInfo),
		potentialServices:  make(pidSet),
		runningServices:    make(pidSet),
		ignorePids:         make(pidSet),
		privilegedDetector: privileged.NewLanguageDetector(),
		scrubber:           procutil.NewDefaultDataScrubber(),
		containerProvider:  containerProvider,
		timeProvider:       tp,
	}
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	sharedContainerProvider := proccontainers.InitSharedContainerProvider(deps.WMeta, deps.Tagger)
	return newDiscovery(sharedContainerProvider, realTime{}), nil
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
	clear(s.ignorePids)
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
	s.mux.Lock()
	defer s.mux.Unlock()

	s.ignorePids[pid] = struct{}{}
}

// shouldIgnorePid returns true if process should be excluded from handling.
func (s *discovery) shouldIgnorePid(pid int32) bool {
	s.mux.Lock()
	defer s.mux.Unlock()

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

	nameMeta := servicediscovery.GetServiceName(lang, ctx)
	apmInstrumentation := apm.Detect(lang, ctx)

	name := nameMeta.DDService
	if name == "" {
		name = nameMeta.Name
	}

	return &serviceInfo{
		name:                name,
		generatedName:       nameMeta.Name,
		generatedNameSource: string(nameMeta.Source),
		ddServiceName:       nameMeta.DDService,
		language:            lang,
		apmInstrumentation:  apmInstrumentation,
		ddServiceInjected:   nameMeta.DDServiceInjected,
		cmdLine:             sanitizeCmdLine(s.scrubber, cmdline),
		startTimeMilli:      uint64(createTime),
	}, nil
}

// maxNumberOfPorts is the maximum number of listening ports which we report per
// service.
const maxNumberOfPorts = 50

// getService gets information for a single service.
func (s *discovery) getService(context parsingContext, pid int32, now time.Time) *model.Service {
	if s.shouldIgnorePid(pid) {
		return nil
	}
	if s.shouldIgnoreComm(pid) {
		s.addIgnoredPid(pid)
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

	if len(ports) > maxNumberOfPorts {
		// Sort the list so that non-ephemeral ports are given preference when
		// we trim the list.
		portCmp := func(a, b uint16) int {
			return cmp.Compare(a, b)
		}
		slices.SortFunc(ports, portCmp)
		ports = ports[:maxNumberOfPorts]
	}

	rss, err := getRSS(pid)
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
		info, err = s.getServiceInfo(pid)
		if err != nil {
			return nil
		}

		s.mux.Lock()
		s.cache[pid] = info
		s.mux.Unlock()
	}

	if s.shouldIgnoreService(info.name) {
		s.addIgnoredPid(pid)
		return nil
	}

	s.mux.Lock()
	info.ports = ports
	info.rss = rss
	if serviceHeartbeatTime := time.Unix(info.lastHeartbeat, 0); now.Sub(serviceHeartbeatTime).Truncate(time.Minute) >= heartbeatTime {
		info.lastHeartbeat = now.Unix()
	}
	s.mux.Unlock()

	service := &model.Service{}
	info.toModelService(pid, service)
	return service
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it.
func (s *discovery) cleanCache(alivePids pidSet) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for pid := range s.cache {
		if alivePids.has(pid) {
			continue
		}

		delete(s.cache, pid)
	}
}

// cleanPidSets deletes dead PIDs from the provided pidSets.
func (s *discovery) cleanPidSets(alivePids pidSet, sets ...pidSet) {
	s.mux.Lock()
	defer s.mux.Unlock()

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
// global CPU time cache for future updates.
func (s *discovery) updateServicesCPUStats(responseServices ...[]model.Service) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if time.Since(s.lastCPUTimeUpdate) < s.config.cpuUsageUpdateDelay {
		return nil
	}

	globalCPUTime, err := getGlobalCPUTime()
	if err != nil {
		return fmt.Errorf("could not get global CPU time: %w", err)
	}

	for _, services := range responseServices {
		for i := range services {
			service := &services[i]
			serviceInfo, ok := s.cache[int32(service.PID)]
			if !ok {
				continue
			}

			_ = updateCPUCoresStats(service.PID, serviceInfo, s.lastGlobalCPUTime, globalCPUTime)
			service.CPUCores = serviceInfo.cpuUsage
		}
	}

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

	for _, tag := range tags {
		// Get index of separator between name and value
		sepIndex := strings.IndexRune(tag, ':')
		if sepIndex < 0 || sepIndex >= len(tag)-1 {
			// Malformed tag; we skip it
			continue
		}

		for i := range tagsPriority {
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

func (s *discovery) enrichContainerData(service *model.Service, containers map[string]*agentPayload.Container, pidToCid map[int]string) {
	id, ok := pidToCid[service.PID]
	if !ok {
		return
	}

	service.ContainerID = id

	// We checked the container tags before, no need to do it again.
	if service.CheckedContainerData {
		return
	}

	container, ok := containers[id]
	if !ok {
		return
	}

	tagName, serviceName := getServiceNameFromContainerTags(container.Tags)
	service.ContainerServiceName = serviceName
	service.ContainerServiceNameSource = tagName
	service.CheckedContainerData = true

	s.mux.Lock()
	serviceInfo, ok := s.cache[int32(service.PID)]
	if ok {
		serviceInfo.containerServiceName = serviceName
		serviceInfo.containerServiceNameSource = tagName
		serviceInfo.checkedContainerData = true
		serviceInfo.containerID = id
	}
	s.mux.Unlock()
}

func (s *discovery) handleStoppedServices(response *model.ServicesResponse, alivePids pidSet) {
	s.mux.Lock()
	defer s.mux.Unlock()

outer:
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
		delete(s.cache, pid)

		for pid := range s.potentialServices {
			potentialInfo, ok := s.cache[pid]
			if !ok {
				log.Warnf("could not get potential service with PID %v from the cache when handling stopped services", pid)
				continue
			}
			if potentialInfo.name == info.name {
				log.Debugf("found potential service with same name as stopped one, skipping end-service event (name: %q)", info.name)
				continue outer
			}
		}

		// Build service struct in place in the slice
		response.StoppedServices = append(response.StoppedServices, model.Service{})
		info.toModelService(pid, &response.StoppedServices[len(response.StoppedServices)-1])
	}
}

// getStatus returns the list of currently running services.
func (s *discovery) getServices() (*model.ServicesResponse, error) {
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
	containers, _, pidToCid, err := s.containerProvider.GetContainers(1*time.Minute, nil)
	if err != nil {
		log.Errorf("could not get containers: %s", err)
	}

	// Build mapping of Container ID to container object to avoid traversal of
	// the containers slice for every services.
	containersMap := make(map[string]*agentPayload.Container, len(containers))
	for _, c := range containers {
		containersMap[c.Id] = c
	}

	now := s.timeProvider.Now()

	for _, pid := range pids {
		alivePids.add(pid)

		service := s.getService(context, pid, now)
		if service == nil {
			continue
		}
		s.enrichContainerData(service, containersMap, pidToCid)

		if _, ok := s.runningServices[pid]; ok {
			if service.LastHeartbeat == now.Unix() {
				response.HeartbeatServices = append(response.HeartbeatServices, *service)
			}
			continue
		}

		if _, ok := s.potentialServices[pid]; ok {
			// We have seen it first in the previous call of getServices, so it
			// is confirmed to be running.
			s.runningServices.add(pid)
			delete(s.potentialServices, pid)
			response.StartedServices = append(response.StartedServices, *service)
			continue
		}

		// This is a new potential service
		s.potentialServices.add(pid)
		log.Debugf("[pid: %d] adding process to potential: %s", pid, service.Name)
	}

	s.handleStoppedServices(response, alivePids)
	s.cleanCache(alivePids)
	s.cleanPidSets(alivePids, s.ignorePids, s.potentialServices)

	if err = s.updateServicesCPUStats(response.StartedServices, response.HeartbeatServices); err != nil {
		log.Warnf("updating services CPU stats: %s", err)
	}

	return response, nil
}
