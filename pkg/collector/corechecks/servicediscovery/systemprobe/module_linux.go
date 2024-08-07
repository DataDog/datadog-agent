// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package systemprobe

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/portlist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	pathOpenPorts = "/open_ports"
	pathGetProc   = "/procs/{pid}"
)

// NewServiceDiscoveryModule creates a new service_discovery system probe module.
func NewServiceDiscoveryModule(_ *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component], _ telemetry.Component) (module.Module, error) {
	poller, err := portlist.NewPoller()
	if err != nil {
		return nil, err
	}
	return &serviceDiscovery{
		portPoller: poller,
	}, nil
}

type serviceDiscovery struct {
	portPoller *portlist.Poller
}

func (s *serviceDiscovery) GetStats() map[string]interface{} {
	return nil
}

func (s *serviceDiscovery) Register(httpMux *module.Router) error {
	httpMux.HandleFunc(pathOpenPorts, s.handleOpenPorts)
	httpMux.HandleFunc(pathGetProc, s.handleGetProc)
	return nil
}

func (s *serviceDiscovery) Close() {}

func (s *serviceDiscovery) handleError(w http.ResponseWriter, route string, status int, err error) {
	_ = log.Errorf("failed to handle /service_discovery/%s (status: %d): %v", route, status, err)
	w.WriteHeader(status)
}

func (s *serviceDiscovery) handleOpenPorts(w http.ResponseWriter, _ *http.Request) {
	ports, err := s.portPoller.OpenPorts()
	if err != nil {
		s.handleError(w, pathOpenPorts, http.StatusInternalServerError, fmt.Errorf("failed to get open ports: %v", err))
		return
	}

	var portsResp []*model.Port
	for _, p := range ports {
		portsResp = append(portsResp, &model.Port{
			PID:         p.Pid,
			ProcessName: p.Process,
			Port:        int(p.Port),
			Proto:       p.Proto,
		})
	}
	resp := &model.OpenPortsResponse{
		Ports: portsResp,
	}
	utils.WriteAsJSON(w, resp)
}

func (s *serviceDiscovery) handleGetProc(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	pidStr := vars["pid"]
	pid, err := strconv.ParseUint(pidStr, 10, 32)
	if err != nil {
		s.handleError(w, pathGetProc, http.StatusBadRequest, fmt.Errorf("failed to convert pid to integer: %v", err))
		return
	}

	if _, err := os.Stat(path.Join(procfs.DefaultMountPoint, pidStr)); os.IsNotExist(err) {
		s.handleError(w, pathGetProc, http.StatusNotFound, fmt.Errorf("/proc/{pid} does not exist: %v", err))
		return
	}
	proc, err := procfs.NewProc(int(pid))
	if err != nil {
		s.handleError(w, pathGetProc, http.StatusInternalServerError, fmt.Errorf("failed to read procfs: %v", err))
		return
	}
	env, err := proc.Environ()
	if err != nil {
		s.handleError(w, pathGetProc, http.StatusInternalServerError, fmt.Errorf("failed to read /proc/{pid}/environ: %v", err))
		return
	}
	cwd, err := proc.Cwd()
	if err != nil {
		s.handleError(w, pathGetProc, http.StatusInternalServerError, fmt.Errorf("failed to read /proc/{pid}/cwd: %v", err))
		return
	}

	env = append(env, getInternalEnvs(int(pid))...)

	resp := &model.GetProcResponse{
		Proc: &model.Proc{
			PID:     int(pid),
			Environ: env,
			CWD:     cwd,
		},
	}
	utils.WriteAsJSON(w, resp)
}

func getInternalEnvs(pid int) []string {
	// add the environment variables in a memory-mapped file in /proc/{PID}/fd/
	// this file is written by the injector (version 0.17.0 and later)
	// if the file isn't there, no worries
	dir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = log.Errorf("failed to read %s: %v", dir, err)
		return nil
	}
	for _, v := range entries {
		realName, err := os.Readlink(dir + "/" + v.Name())
		if err != nil {
			_ = log.Errorf("error reading link %s: %v", v.Name(), err)
			continue
		}
		if strings.HasPrefix(realName, "/memfd:envs") {
			file, err := os.ReadFile(dir + "/" + v.Name())
			if err != nil {
				_ = log.Errorf("error reading file %s: %v", v.Name(), err)
				continue
			}
			return strings.Split(strings.TrimSpace(string(file)), "\n")
		}
	}
	return nil
}
