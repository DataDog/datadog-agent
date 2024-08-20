// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type workloadmeta struct {
	log    log.Component
	config config.Component

	// Store related
	storeMut sync.RWMutex
	store    map[wmdef.Kind]map[string]*cachedEntity // store[entity.Kind][entity.ID] = &cachedEntity{}

	subscribersMut sync.RWMutex
	subscribers    []subscriber

	collectorMut sync.RWMutex
	candidates   map[string]wmcatalog.Collector
	collectors   map[string]wmcatalog.Collector

	eventCh chan []wmdef.CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started
}

// Dependencies defines the dependencies of the workloadmeta component.
type Dependencies struct {
	Lc compdef.Lifecycle

	Log     log.Component
	Config  config.Component
	Catalog []wmcatalog.Collector `group:"workloadmeta"`

	Params wmdef.Params
}

// Provider contains components provided by workloadmeta constructor.
type Provider struct {
	Comp          wmdef.Component
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

// NewWorkloadMeta creates a new workloadmeta component.
func NewWorkloadMeta(deps Dependencies) Provider {
	candidates := make(map[string]wmcatalog.Collector)
	for _, c := range fxutil.GetAndFilterGroup(deps.Catalog) {
		if (c.GetTargetCatalog() & deps.Params.AgentType) > 0 {
			candidates[c.GetID()] = c
		}
	}

	wm := &workloadmeta{
		log:    deps.Log,
		config: deps.Config,

		store:        make(map[wmdef.Kind]map[string]*cachedEntity),
		candidates:   candidates,
		collectors:   make(map[string]wmcatalog.Collector),
		eventCh:      make(chan []wmdef.CollectorEvent, eventChBufferSize),
		ongoingPulls: make(map[string]time.Time),
	}

	deps.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {

		var err error

		// Main context passed to components
		// TODO(components): this mainCtx should probably be replaced by the
		//                   context provided to the OnStart hook.
		mainCtx, _ := common.GetMainCtxCancel()

		if deps.Params.InitHelper != nil {
			err = deps.Params.InitHelper(mainCtx, wm, deps.Config)
			if err != nil {
				return err
			}
		}
		wm.start(mainCtx)
		return nil
	}})
	deps.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		// TODO(components): workloadmeta should probably be stopped cleanly
		return nil
	}})

	return Provider{
		Comp:          wm,
		FlareProvider: flaretypes.NewProvider(wm.sbomFlareProvider),
		Endpoint:      api.NewAgentEndpointProvider(wm.writeResponse, "/workload-list", "GET"),
	}
}

func (w *workloadmeta) writeResponse(writer http.ResponseWriter, r *http.Request) {
	verbose := false
	params := r.URL.Query()
	if v, ok := params["verbose"]; ok {
		if len(v) >= 1 && v[0] == "true" {
			verbose = true
		}
	}

	response := w.Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(writer, w.log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	writer.Write(jsonDump)
}
