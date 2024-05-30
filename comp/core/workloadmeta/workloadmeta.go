// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"encoding/base64"
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
		Comp:                 wm,
		FlareProvider:        flaretypes.NewProvider(wm.sbomFlareProvider),
		WorkloadListEndpoint: api.NewAgentEndpointProvider(wm.writeResponse, "/workload-list", "GET"),

		PodContainerMetadataEndpoint: api.NewAgentEndpointProvider(
			PodContainerMetadataHandler(wm, wm.log),
			"/pod-container-metadata",
			"GET",
		),
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

type PodContainerMetadataResponse struct {
	Containers map[string]PodContainerMetadata `json:"containers"`
}

type PodContainerMetadata struct {
	Name              string   `json:"name"`
	InitContainerName string   `json:"initContainerName"`
	Cmd               []string `json:"cmd"`
}

type ContainerSpec struct {
	ContainerName string   `json:"containerName"`
	Command       []string `json:"command"`
	Args          []string `json:"args"`
}

func (c ContainerSpec) determineCmd(i *ContainerImageMetadata) []string {
	var out []string
	if len(c.Command) != 0 {
		out = c.Command
	} else if len(i.Entrypoint) > 0 {
		out = i.Entrypoint
	}

	if len(c.Args) > 0 {
		out = append(out, c.Args...)
	} else if len(i.Cmd) > 0 {
		out = append(out, i.Cmd...)
	}

	return out
}

type PodContainerMetadataRequest struct {
	PodName        string
	PodNamespace   string
	InitContainers map[string]ContainerSpec `json:"initContainers"`
}

func GetPodContainerMetadata(wm Component, log log.Component, r PodContainerMetadataRequest) (PodContainerMetadataResponse, error) {
	if r.PodName == "" || r.PodNamespace == "" {
		return PodContainerMetadataResponse{}, fmt.Errorf("missing pod name or namespaces")
	}

	pod, err := wm.GetKubernetesPodByName(r.PodName, r.PodNamespace)
	if err != nil {
		return PodContainerMetadataResponse{}, err
	}

	allImages := wm.ListImages()
	findImage := func(name string) *ContainerImageMetadata {
		for _, i := range allImages {
			if i.ID == name {
				return i
			}
			for _, digest := range i.RepoDigests {
				if digest == name {
					return i
				}
			}
		}

		return nil
	}

	out := PodContainerMetadataResponse{
		Containers: map[string]PodContainerMetadata{},
	}

	for _, c := range pod.InitContainers {
		spec, ok := r.InitContainers[c.Name]
		if !ok {
			continue // we don't care about this
		}

		image := findImage(c.Image.ImageMetadataID())
		if image != nil {
			cmd := spec.determineCmd(image)
			if len(cmd) == 0 {
				return out, fmt.Errorf("could not determine command for container %s", c.Name)
			}

			out.Containers[spec.ContainerName] = PodContainerMetadata{
				InitContainerName: c.Name,
				Name:              spec.ContainerName,
				Cmd:               cmd,
			}
			continue
		}

		return out, fmt.Errorf("could not get image for container %s", c.Name)
	}

	if len(r.InitContainers) != len(out.Containers) {
		return out, fmt.Errorf("missing container metadata, try again, expected %v", mapKeys(r.InitContainers))
	}

	return out, nil
}

func mapKeys[T map[K]V, K comparable, V any](in T) []K {
	keys := make([]K, len(in))

	i := 0
	for k := range in{
		keys[i] = k
		i++
	}

	return keys
}

func PodContainerMetadataHandler(wm Component, log log.Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			q    = r.URL.Query()
			name = q.Get("name")
			ns   = q.Get("ns")
			rb64 = q.Get("request")
		)

		rbytes, err := base64.StdEncoding.DecodeString(rb64)
		if err != nil {
			utils.SetJSONError(w, log.Errorf("error decoding request payload"), 400)
			return
		}

		var initContainers map[string]ContainerSpec
		err = json.Unmarshal(rbytes, &initContainers)
		if err != nil {
			utils.SetJSONError(w, log.Errorf("invalid encoding"), 400)
			return
		}

		output, err := GetPodContainerMetadata(wm, log, PodContainerMetadataRequest{
			PodName:        name,
			PodNamespace:   ns,
			InitContainers: initContainers,
		})
		if err != nil {
			utils.SetJSONError(w, log.Errorf("error fetching pod container metadata for pod=%s/%s: %v", ns, name, err), 500)
			return
		}

		jsonDump, err := json.Marshal(output)
		if err != nil {
			utils.SetJSONError(w, log.Errorf("unable to marshal pod-container-metadata: %v", err), 500)
			return
		}

		_, _ = w.Write(jsonDump)
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
