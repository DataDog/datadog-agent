// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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

	collectorMut          sync.RWMutex
	candidates            map[string]wmdef.Collector
	collectors            map[string]wmdef.Collector
	collectorsInitialized wmdef.CollectorStatus

	eventCh chan []wmdef.CollectorEvent

	ongoingPullsMut sync.Mutex
	ongoingPulls    map[string]time.Time // collector ID => time when last pull started
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

	jsonDump, err := BuildWorkloadResponse(
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

// BuildWorkloadResponse builds a JSON response for workload-list with filtering.
//
// Backend does all processing to avoid client-side unmarshaling issues:
//  1. Get structured entities (DumpStructured returns concrete types from workloadmeta store)
//  2. Apply search filtering on structured data (single filtering function)
//  3. Convert to requested format:
//     - jsonFormat=true: Return structured JSON (for -j/-p flags)
//     - jsonFormat=false: Convert to text format using entity.String(verbose)
//
// This approach leverages the backend's access to concrete entity types, avoiding the
// unmarshaling problem where clients can't reconstruct interface types from JSON.
func BuildWorkloadResponse(wmeta wmdef.Component, verbose bool, search string, jsonFormat bool) ([]byte, error) {
	// Get structured data from workloadmeta store (has concrete entity types)
	structuredResp := wmeta.DumpStructured(verbose)

	// Apply search filter on structured data (single filtering logic - no duplication!)
	if search != "" {
		structuredResp = FilterStructuredResponse(structuredResp, search)
	}

	// Backend decides output format based on client request
	if jsonFormat {
		// Return structured JSON for JSON display (-j or -p flags)
		return json.Marshal(structuredResp)
	}

	// Convert to text format for text display (no flags)
	// This conversion happens here because backend has concrete types
	textResp := convertStructuredToText(structuredResp, verbose)
	return json.Marshal(textResp)
}

// convertStructuredToText converts structured entities to text format by calling String(verbose).
// Note: This simplified conversion doesn't preserve individual source information that Dump() shows
// in verbose mode, as DumpStructured() returns merged entities only. The merged entity (which combines
// data from all sources) is sufficient for most use cases.
func convertStructuredToText(structured wmdef.WorkloadDumpStructuredResponse, verbose bool) wmdef.WorkloadDumpResponse {
	textResp := wmdef.WorkloadDumpResponse{
		Entities: make(map[string]wmdef.WorkloadEntity),
	}

	for kind, entities := range structured.Entities {
		infos := make(map[string]string)
		for _, entity := range entities {
			// Use entity ID as key (simpler than Dump's "sources(merged):[...] id: xyz" format)
			infos[entity.GetID().ID] = entity.String(verbose)
		}
		if len(infos) > 0 {
			textResp.Entities[kind] = wmdef.WorkloadEntity{Infos: infos}
		}
	}

	return textResp
}

// FilterStructuredResponse filters entities by kind or entity ID
func FilterStructuredResponse(response wmdef.WorkloadDumpStructuredResponse, search string) wmdef.WorkloadDumpStructuredResponse {
	filtered := wmdef.WorkloadDumpStructuredResponse{
		Entities: make(map[string][]wmdef.Entity),
	}

	for kind, entities := range response.Entities {
		if strings.Contains(kind, search) {
			// Kind matches - include all entities
			filtered.Entities[kind] = entities
			continue
		}

		// Filter by entity ID
		var matchingEntities []wmdef.Entity
		for _, entity := range entities {
			if strings.Contains(entity.GetID().ID, search) {
				matchingEntities = append(matchingEntities, entity)
			}
		}

		if len(matchingEntities) > 0 {
			filtered.Entities[kind] = matchingEntities
		}
	}

	return filtered
}

