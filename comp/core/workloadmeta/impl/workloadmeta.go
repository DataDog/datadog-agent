// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

//TODO(components): remove fx from this package to follow the new component layout

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
	candidates   map[string]wmdef.Collector
	collectors   map[string]wmdef.Collector

	eventCh chan []wmdef.CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log     log.Component
	Config  config.Component
	Catalog wmdef.CollectorList `group:"workloadmeta"`

	Params wmdef.Params
}

// Provider contains components provided by workloadmeta constructor.
type Provider struct {
	fx.Out

	Comp          wmdef.Component
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

// OptionalProvider contains components provided by workloadmeta optional constructor.
type OptionalProvider struct {
	fx.Out

	Comp          optional.Option[wmdef.Component]
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

// NewWorkloadMeta creates a new workloadmeta component.
func NewWorkloadMeta(deps dependencies) Provider {
	candidates := make(map[string]wmdef.Collector)
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
		collectors:   make(map[string]wmdef.Collector),
		eventCh:      make(chan []wmdef.CollectorEvent, eventChBufferSize),
		ongoingPulls: make(map[string]time.Time),
	}

	deps.Lc.Append(fx.Hook{OnStart: func(c context.Context) error {

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
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error {
		// TODO(components): workloadmeta should probably be stopped cleanly
		return nil
	}})

	return Provider{
		Comp:          wm,
		FlareProvider: flaretypes.NewProvider(wm.sbomFlareProvider),
		Endpoint:      api.NewAgentEndpointProvider(wm.writeResponse, "/workload-list", "GET"),
	}
}

// NewWorkloadMetaOptional creates a new optional workloadmeta component.
func NewWorkloadMetaOptional(deps dependencies) OptionalProvider {
	if deps.Params.NoInstance {
		return OptionalProvider{
			Comp: optional.NewNoneOption[wmdef.Component](),
		}
	}
	c := NewWorkloadMeta(deps)

	return OptionalProvider{
		Comp:          optional.NewOption(c.Comp),
		FlareProvider: c.FlareProvider,
		Endpoint:      c.Endpoint,
	}
}

func (wm *workloadmeta) writeResponse(w http.ResponseWriter, r *http.Request) {
	verbose := false
	params := r.URL.Query()
	if v, ok := params["verbose"]; ok {
		if len(v) >= 1 && v[0] == "true" {
			verbose = true
		}
	}

	response := wm.Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		utils.SetJSONError(w, wm.log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}
