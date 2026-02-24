// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
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
