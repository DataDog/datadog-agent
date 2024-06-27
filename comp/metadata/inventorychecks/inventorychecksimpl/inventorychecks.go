// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventorychecksimpl implements the inventorychecks component interface.
package inventorychecksimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newInventoryChecksProvider),
	)
}

type metadata map[string]interface{}
type checksMetadata map[string][]metadata

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname     string                `json:"hostname"`
	Timestamp    int64                 `json:"timestamp"`
	Metadata     map[string][]metadata `json:"check_metadata"`
	LogsMetadata map[string][]metadata `json:"logs_metadata"`
	UUID         string                `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories host payload any more, payload is too big for intake")
}

type instanceMetadata struct {
	LastUpdated time.Time
	instanceID  string
	metadata    metadata
}

type inventorychecksImpl struct {
	util.InventoryPayload

	m sync.Mutex
	// data is a map of instanceID to metadata
	data map[string]instanceMetadata

	log      log.Component
	conf     config.Component
	coll     optional.Option[collector.Component]
	sources  optional.Option[*sources.LogSources]
	hostname string
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Coll       optional.Option[collector.Component]
	LogAgent   optional.Option[logagent.Component]
}

type provides struct {
	fx.Out

	Comp          inventorychecks.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func newInventoryChecksProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	ic := &inventorychecksImpl{
		conf:     deps.Config,
		log:      deps.Log,
		coll:     deps.Coll,
		sources:  optional.NewNoneOption[*sources.LogSources](),
		hostname: hname,
		data:     map[string]instanceMetadata{},
	}
	ic.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, ic.getPayload, "checks.json")

	// We want to be notified when the collector add or removed a check.
	// TODO: (component) - This entire metadata provider should be part of the collector. Once the collector is a
	// component we can migrate it there and remove the entire logic to emit event from the collector.

	if coll, isSet := ic.coll.Get(); isSet {
		coll.AddEventReceiver(func(_ checkid.ID, _ collector.EventType) { ic.Refresh() })
	}

	if logAgent, isSet := deps.LogAgent.Get(); isSet {
		ic.sources.Set(logAgent.GetSources())
	}

	return provides{
		Comp:          ic,
		Provider:      ic.MetadataProvider(),
		FlareProvider: ic.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(ic.writePayloadAsJSON, "/metadata/inventory-checks", "GET"),
	}
}

// Set sets a metadata value for one check instance
func (ic *inventorychecksImpl) Set(instanceID string, key string, value interface{}) {
	if !ic.Enabled || instanceID == "" {
		return
	}

	ic.log.Debugf("setting check metadata for check %s, '%s': '%s'", instanceID, key, value)

	ic.m.Lock()
	defer ic.m.Unlock()

	entry, found := ic.data[instanceID]
	if !found {
		entry = instanceMetadata{
			instanceID: instanceID,
			metadata:   map[string]interface{}{},
		}
		ic.data[instanceID] = entry
	}

	if !reflect.DeepEqual(entry.metadata[key], value) {
		entry.LastUpdated = time.Now()
		entry.metadata[key] = value

		ic.Refresh()
	}
}

func (ic *inventorychecksImpl) GetInstanceMetadata(instanceID string) map[string]interface{} {
	ic.m.Lock()
	defer ic.m.Unlock()

	res := map[string]interface{}{}
	if instance, found := ic.data[instanceID]; found {
		for name, value := range instance.metadata {
			res[name] = value
		}
	}
	return res
}

func (ic *inventorychecksImpl) getPayload() marshaler.JSONMarshaler {
	ic.m.Lock()
	defer ic.m.Unlock()

	payloadData := make(checksMetadata)
	invChecksEnabled := ic.conf.GetBool("inventories_checks_configuration_enabled")

	if coll, isSet := ic.coll.Get(); isSet {
		foundInCollector := map[string]struct{}{}

		coll.MapOverChecks(func(checks []check.Info) {
			for _, c := range checks {
				cm := check.GetMetadata(c, invChecksEnabled)

				if checkData, found := ic.data[string(c.ID())]; found {
					for key, val := range checkData.metadata {
						cm[key] = val
					}
				}

				checkName := c.String()
				payloadData[checkName] = append(payloadData[checkName], cm)

				instanceID := string(c.ID())
				foundInCollector[instanceID] = struct{}{}
			}
		})

		// if metadata were added for a check not in the collector we clear the cache. This can happen when a check
		// submit metadata after being unscheduled but before exiting its last run.
		for instanceID := range ic.data {
			if _, found := foundInCollector[instanceID]; !found {
				delete(ic.data, instanceID)
			}
		}
	}

	logsMetadata := make(map[string][]metadata)
	if sources, isSet := ic.sources.Get(); isSet && invChecksEnabled {
		if sources != nil {
			for _, logSource := range sources.GetSources() {
				if _, found := logsMetadata[logSource.Name]; !found {
					logsMetadata[logSource.Name] = []metadata{}
				}

				parsedJSON, err := logSource.Config.PublicJSON()
				if err != nil {
					ic.log.Debugf("could not parse log configuration for source metadata %s: %v", logSource.Name, err)
					continue
				}

				tags := logSource.Config.Tags
				if tags == nil {
					tags = []string{}
				}
				logsMetadata[logSource.Name] = append(logsMetadata[logSource.Name], metadata{
					"config": string(parsedJSON),
					"state": map[string]string{
						"error":  logSource.Status.GetError(),
						"status": logSource.Status.String(),
					},
					"service":          logSource.Config.Service,
					"source":           logSource.Config.Source,
					"integration_name": logSource.Config.IntegrationName,
					"tags":             tags,
				})
			}
		}
	}

	return &Payload{
		Hostname:     ic.hostname,
		Timestamp:    time.Now().UnixNano(),
		Metadata:     payloadData,
		LogsMetadata: logsMetadata,
		UUID:         uuid.GetUUID(),
	}
}

func (ic *inventorychecksImpl) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := ic.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
