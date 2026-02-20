// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
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

	collectorMut          sync.RWMutex
	candidates            map[string]wmdef.Collector
	collectors            map[string]wmdef.Collector
	collectorsInitialized wmdef.CollectorStatus

	eventCh chan []wmdef.CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started

	// expectedSources maps entity kinds to the sources that are expected to
	// report data for them. This is used to determine if an entity is
	// "complete" (all expected collectors have reported).
	//
	// TODO: For now, this map is static and not updated when a collector
	// permanently fails. A permanent failure means entities waiting on that
	// source will never be considered complete.
	expectedSources map[wmdef.Kind][]wmdef.Source
}

// Dependencies defines the dependencies of the workloadmeta component.
type Dependencies struct {
	Lc compdef.Lifecycle

	Log     log.Component
	Config  config.Component
	Catalog wmdef.CollectorList `group:"workloadmeta"`

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
	candidates := make(map[string]wmdef.Collector)
	for _, c := range fxutil.GetAndFilterGroup(deps.Catalog) {
		if (c.GetTargetCatalog() & deps.Params.AgentType) > 0 {
			candidates[c.GetID()] = c
		}
	}

	wm := &workloadmeta{
		log:    deps.Log,
		config: deps.Config,

		store:                 make(map[wmdef.Kind]map[string]*cachedEntity),
		candidates:            candidates,
		collectors:            make(map[string]wmdef.Collector),
		eventCh:               make(chan []wmdef.CollectorEvent, eventChBufferSize),
		ongoingPulls:          make(map[string]time.Time),
		collectorsInitialized: wmdef.CollectorsNotStarted,
		expectedSources:       initExpectedSources(),
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
	params := r.URL.Query()

	jsonDump, err := wmdef.BuildWorkloadResponse(
		w,
		params.Get("verbose") == "true",
		params.Get("search"),
		params.Get("format") == "json",
	)
	if err != nil {
		httputils.SetJSONError(writer, w.log.Errorf("Unable to build workload list response: %v", err), 500)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(jsonDump)
}

// initExpectedSources initializes the expected sources map based on the
// detected environment features. This determines which collectors are expected
// to report data for each entity kind.
//
// For now, it only supports Kubernetes. Other cases with multiple collectors
// reporting the same entity kind:
// - ECS EC2: containers are reported by both the ECS and Docker collectors.
// TODO: This will be handled later.
// - Kubernetes Deployments: reported by the kubeapiserver collector and
// language detection code. Completeness tracking is not needed for deployments.
func initExpectedSources() map[wmdef.Kind][]wmdef.Source {
	expectedSources := make(map[wmdef.Kind][]wmdef.Source)

	if !env.IsFeaturePresent(env.Kubernetes) {
		return expectedSources
	}

	// In Kubernetes, pods are reported by:
	// - kubelet collector (SourceNodeOrchestrator)
	// - kubemetadata collector (SourceClusterOrchestrator)
	expectedSources[wmdef.KindKubernetesPod] = []wmdef.Source{
		wmdef.SourceNodeOrchestrator,
		wmdef.SourceClusterOrchestrator,
	}

	// In Kubernetes, containers are reported by:
	// - kubelet collector (SourceNodeOrchestrator)
	// - container runtime collector if accessible (SourceRuntime)
	containerSources := []wmdef.Source{wmdef.SourceNodeOrchestrator}
	if containerRuntimeIsAccessible() {
		containerSources = append(containerSources, wmdef.SourceRuntime)
	}
	expectedSources[wmdef.KindContainer] = containerSources

	return expectedSources
}

func containerRuntimeIsAccessible() bool {
	runtimes := []env.Feature{env.Docker, env.Containerd, env.Crio, env.Podman}
	return slices.ContainsFunc(runtimes, env.IsFeaturePresent)
}
