// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/portlist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	pathOpenPorts  = "/open_ports"
	pathPIDEnviron = "/{pid}/environ"
)

// ServiceDiscoveryModule is the language detection module factory
var ServiceDiscoveryModule = module.Factory{
	Name:             config.ServiceDiscoveryModule,
	ConfigNamespaces: []string{"service_discovery"},
	Fn: func(cfg *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component], _ telemetry.Component) (module.Module, error) {
		// return module.ErrNotEnabled if should not be initialized

		poller, err := portlist.NewPoller()
		if err != nil {
			return nil, err
		}
		return &serviceDiscovery{
			portPoller: poller,
		}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

type serviceDiscovery struct {
	portPoller *portlist.Poller
}

func (s *serviceDiscovery) GetStats() map[string]interface{} {
	return nil
}

func (s *serviceDiscovery) Register(router *module.Router) error {
	router.HandleFunc(pathOpenPorts, s.handleOpenPorts)
	router.HandleFunc(pathPIDEnviron, s.handlePIDEnviron)
	return nil
}

// Close closes resources associated with the language detection module.
// The language detection module doesn't do anything except route to the privileged language detection api.
// This API currently does not hold any resources over its lifetime, so there is no need to release any resources when the
// module is closed.

func (s *serviceDiscovery) Close() {}

func (s *serviceDiscovery) handleError(w http.ResponseWriter, route string, status int, err error) {
	_ = log.Errorf("failed to handle /service_discovery/%s (status: %d): %v", route, status, err)
	w.WriteHeader(status)
}

func (s *serviceDiscovery) handleOpenPorts(w http.ResponseWriter, req *http.Request) {
	ports, err := s.portPoller.OpenPorts()
	if err != nil {
		s.handleError(w, pathOpenPorts, http.StatusInternalServerError, fmt.Errorf("failed to get open ports: %v", err))
	}

	utils.WriteAsJSON(w, ports)
}

func (s *serviceDiscovery) handlePIDEnviron(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	pid, err := strconv.ParseUint(vars["pid"], 10, 32)
	if err != nil {
		s.handleError(w, pathPIDEnviron, http.StatusBadRequest, fmt.Errorf("failed to convert pid to integer: %v", err))
		return
	}

	proc, err := procfs.NewProc(int(pid))
	if err != nil {
		s.handleError(w, pathPIDEnviron, http.StatusInternalServerError, fmt.Errorf("failed to read procfs: %v", err))
	}

	env, err := proc.Environ()
	if err != nil {
		s.handleError(w, pathPIDEnviron, http.StatusInternalServerError, fmt.Errorf("failed to read /proc/{pid}/environ: %v", err))
	}

	utils.WriteAsJSON(w, env)
}
