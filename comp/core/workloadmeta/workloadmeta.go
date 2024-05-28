// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type workloadmeta struct {
	log    log.Component
	config config.Component

	// Store related
	storeMut sync.RWMutex
	store    map[Kind]map[string]*cachedEntity // store[entity.Kind][entity.ID] = &cachedEntity{}

	subscribersMut sync.RWMutex
	subscribers    []subscriber

	collectorMut sync.RWMutex
	candidates   map[string]Collector
	collectors   map[string]Collector

	eventCh chan []CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started
}

// InitHelper this should be provided as a helper to allow passing the component into
// the inithook for additional start-time configutation.
type InitHelper func(context.Context, Component, config.Component) error

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log     log.Component
	Config  config.Component
	Catalog CollectorList `group:"workloadmeta"`

	Params Params
}

type provider struct {
	fx.Out

	Comp                         Component
	FlareProvider                flaretypes.Provider
	WorkloadListEndpoint         api.AgentEndpointProvider
	PodContainerMetadataEndpoint api.AgentEndpointProvider
}

type optionalProvider struct {
	fx.Out

	Comp                         optional.Option[Component]
	FlareProvider                flaretypes.Provider
	WorkloadListEndpoint         api.AgentEndpointProvider
	PodContainerMetadataEndpoint api.AgentEndpointProvider
}

func newWorkloadMeta(deps dependencies) provider {
	candidates := make(map[string]Collector)
	for _, c := range fxutil.GetAndFilterGroup(deps.Catalog) {
		if (c.GetTargetCatalog() & deps.Params.AgentType) > 0 {
			candidates[c.GetID()] = c
		}
	}

	wm := &workloadmeta{
		log:    deps.Log,
		config: deps.Config,

		store:        make(map[Kind]map[string]*cachedEntity),
		candidates:   candidates,
		collectors:   make(map[string]Collector),
		eventCh:      make(chan []CollectorEvent, eventChBufferSize),
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

	return provider{
		Comp:                         wm,
		FlareProvider:                flaretypes.NewProvider(wm.sbomFlareProvider),
		WorkloadListEndpoint:         api.NewAgentEndpointProvider(wm.writeResponse, "/workload-list", "GET"),
		PodContainerMetadataEndpoint: api.NewAgentEndpointProvider(wm.podContainerMetadataHandler, "/pod-container-metadata", "GET"),
	}
}

func newWorkloadMetaOptional(deps dependencies) optionalProvider {
	if deps.Params.NoInstance {
		return optionalProvider{
			Comp: optional.NewNoneOption[Component](),
		}
	}
	c := newWorkloadMeta(deps)

	return optionalProvider{
		Comp:                         optional.NewOption(c.Comp),
		FlareProvider:                c.FlareProvider,
		WorkloadListEndpoint:         c.WorkloadListEndpoint,
		PodContainerMetadataEndpoint: c.PodContainerMetadataEndpoint,
	}
}

type PodContainerMetadata struct {
	Name       string         `json:"name"`
	Cmd        []string       `json:"cmd"`
	Entrypoint []string       `json:"entrypoint"`
	Image      ContainerImage `json:"image"`
}

func (wm *workloadmeta) podContainerMetadata(name, ns string) (map[string]PodContainerMetadata, error) {
	if ns == "" || name == "" {
		return nil, fmt.Errorf("missing pod name or namespaces")
	}

	pod, err := wm.GetKubernetesPodByName(name, ns)
	if err != nil {
		return nil, err
	}

	out := map[string]PodContainerMetadata{}
	for _, c := range pod.Containers {
		image, err := wm.GetImage(c.Image.ImageMetadataID())
		if err != nil {
			return out, fmt.Errorf("could not get image for container %s: %w", c.Name, err)
		}

		out[c.Name] = PodContainerMetadata{
			Name:       c.Name,
			Image:      c.Image,
			Cmd:        image.Cmd,
			Entrypoint: image.Entrypoint,
		}
	}

	return out, nil
}

func (wm *workloadmeta) podContainerMetadataHandler(w http.ResponseWriter, r *http.Request) {
	var (
		q    = r.URL.Query()
		name = q.Get("name")
		ns   = q.Get("ns")
	)

	output, err := wm.podContainerMetadata(name, ns)
	if err != nil {
		utils.SetJSONError(w, wm.log.Errorf("error fetching pod container metadata for pod=%s/%s: %v", ns, name, err), 500)
		return
	}

	jsonDump, err := json.Marshal(output)
	if err != nil {
		utils.SetJSONError(w, wm.log.Errorf("unable to marshal pod-container-metadata: %v", err), 500)
		return
	}

	w.Write(jsonDump)
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
