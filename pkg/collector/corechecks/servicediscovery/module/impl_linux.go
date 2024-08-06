// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	pathServices = "/services"
)

// Ensure discovery implements the module.Module interface.
var _ module.Module = &discovery{}

// cacheData holds process data that should be cached between calls to the
// endpoint.
type cacheData struct {
	serviceName string
}

// discovery is an implementation of the Module interface for the discovery module.
type discovery struct {
	// cache maps pids to data that should be cached between calls to the endpoint.
	cache           map[int32]cacheData
	serviceDetector servicediscovery.ServiceDetector
}

// NewDiscoveryModule creates a new discovery system probe module.
func NewDiscoveryModule(*sysconfigtypes.Config, optional.Option[workloadmeta.Component], telemetry.Component) (module.Module, error) {
	return &discovery{
		cache:           make(map[int32]cacheData),
		serviceDetector: *servicediscovery.NewServiceDetector(),
	}, nil
}

// GetStats returns the stats of the discovery module.
func (s *discovery) GetStats() map[string]interface{} {
	return nil
}

// Register registers the discovery module with the provided HTTP mux.
func (s *discovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/status", s.handleStatusEndpoint)
	httpMux.HandleFunc(pathServices, s.handleServices)
	return nil
}

// Close cleans resources used by the discovery module.
func (s *discovery) Close() {
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

// getSockets get a list of socket inode numbers opened by a process. Based on
// snapshotBoundSockets() in
// pkg/security/security_profile/activity_tree/process_node_snapshot.go. The
// socket inode information from /proc/../fd is needed to map the connection
// from the net/tcp (and similar) files to actual ports.
func getSockets(p *process.Process) ([]uint64, error) {
	FDs, err := p.OpenFiles()
	if err != nil {
		return nil, err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		const prefix = "socket:["
		if strings.HasPrefix(fd.Path, prefix) {
			inodeStr := strings.TrimPrefix(fd.Path[:len(fd.Path)-1], prefix)
			sock, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
}

// namespaceInfo stores information related to each network namespace.
type namespaceInfo struct {
	// listeningSockets stores the socket inode numbers for sockets which are listening.
	listeningSockets map[uint64]struct{}
}

// Lifted from pkg/network/proc_net.go
const (
	tcpListen uint64 = 10

	// tcpClose is also used to indicate a UDP connection where the other end hasn't been established
	tcpClose  uint64 = 7
	udpListen        = tcpClose
)

// addSockets adds only listening sockets to a map (set) to be used for later looksups.
func addSockets[P procfs.NetTCP | procfs.NetUDP](sockMap map[uint64]struct{}, sockets P, state uint64) {
	for _, sock := range sockets {
		if sock.St != state {
			continue
		}
		sockMap[sock.Inode] = struct{}{}
	}
}

// getNsInfo gets the list of open ports with socket inodes for all supported
// protocols for the provided namespace. Based on snapshotBoundSockets() in
// pkg/security/security_profile/activity_tree/process_node_snapshot.go.
func getNsInfo(pid int) (*namespaceInfo, error) {
	path := kernel.HostProc(fmt.Sprintf("%d", pid))
	proc, err := procfs.NewFS(path)
	if err != nil {
		log.Warnf("error while opening procfs (pid: %v): %s", pid, err)
		return nil, err
	}

	TCP, err := proc.NetTCP()
	if err != nil {
		log.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	UDP, err := proc.NetUDP()
	if err != nil {
		log.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	TCP6, err := proc.NetTCP6()
	if err != nil {
		log.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	UDP6, err := proc.NetUDP6()
	if err != nil {
		log.Debugf("couldn't snapshot UDP6 sockets: %v", err)
	}

	listeningSockets := make(map[uint64]struct{})

	addSockets(listeningSockets, TCP, tcpListen)
	addSockets(listeningSockets, TCP6, tcpListen)
	addSockets(listeningSockets, UDP, udpListen)
	addSockets(listeningSockets, UDP6, udpListen)

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

// getServiceName gets the service name for a process using the servicedetector
// module.
func (s *discovery) getServiceName(proc *process.Process) (string, error) {
	cmdline, err := proc.CmdlineSlice()
	if err != nil {
		return "", nil
	}

	env, err := proc.Environ()
	if err != nil {
		return "", nil
	}

	return s.serviceDetector.GetServiceName(cmdline, env), nil
}

// getService gets information for a single service.
func (s *discovery) getService(context parsingContext, pid int32) *model.Service {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	sockets, err := getSockets(proc)
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

	haveListeningSocket := false
	for _, socket := range sockets {
		if _, ok := nsInfo.listeningSockets[socket]; ok {
			haveListeningSocket = true
			break
		}
	}

	if !haveListeningSocket {
		return nil
	}

	var serviceName string
	if cached, ok := s.cache[pid]; ok {
		serviceName = cached.serviceName
	} else {
		serviceName, err = s.getServiceName(proc)
		if err != nil {
			return nil
		}

		s.cache[pid] = cacheData{serviceName: serviceName}
	}

	return &model.Service{
		PID:  int(pid),
		Name: serviceName,
	}
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it.
func (s *discovery) cleanCache(alivePids map[int32]struct{}) {
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
