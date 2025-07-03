// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryotelimpl implements a component to generate the 'datadog_agent' metadata payload for inventory.
package inventoryotelimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	iointerface "github.com/DataDog/datadog-agent/comp/metadata/inventoryotel"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newInventoryOtelProvider))
}

type otelMetadata = map[string]interface{}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string       `json:"hostname"`
	Timestamp int64        `json:"timestamp"`
	Metadata  otelMetadata `json:"otel_metadata"`
	UUID      string       `json:"uuid"`
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
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
}

type inventoryotel struct {
	util.InventoryPayload

	conf     config.Component
	log      log.Component
	m        sync.Mutex
	data     otelMetadata
	hostname string
	client   ipc.HTTPClient
	f        *freshConfig
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Client     ipc.HTTPClient
	Hostname   hostnameinterface.Component
}

type provides struct {
	fx.Out

	Comp          iointerface.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func newInventoryOtelProvider(deps dependencies) (provides, error) {
	hname, _ := deps.Hostname.Get(context.Background())
	i := &inventoryotel{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     make(otelMetadata),
		client:   deps.Client,
	}

	getter := i.fetchRemoteOtelConfig
	if i.conf.GetBool("otelcollector.submit_dummy_metadata") {
		getter = i.fetchDummyOtelConfig
	}

	var err error
	i.f, err = newFreshConfig(deps.Config.GetString("otelcollector.extension_url"), getter)
	if err != nil {
		// panic?
		return provides{}, err
	}

	i.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, i.getPayload, "otel.json")

	if i.Enabled {
		// TODO: if there's an update on the OTel side we currently will not be
		//       notified. Is this a problem? Runtime changes are expected to be
		//       triggered by FA, so maybe this is OK.
		//
		// We want to be notified when the configuration is updated
		deps.Config.OnUpdate(func(_ string, _, _ any, _ uint64) { i.Refresh() })
	}

	return provides{
		Comp:          i,
		Provider:      i.MetadataProvider(),
		FlareProvider: i.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(i.writePayloadAsJSON, "/metadata/inventory-otel", "GET"),
	}, nil
}

func (i *inventoryotel) parseResponseFromJSON(body []byte) (otelMetadata, error) {
	var c interface{}

	err := json.Unmarshal(body, &c)
	if err != nil {
		i.log.Error("Error unmarshaling the response:", err)
		return nil, err
	}

	conf := c.(otelMetadata)

	// Sources and environment are not relevant for the metadata payload
	delete(conf, "sources")
	delete(conf, "environment")

	return conf, nil
}

func (i *inventoryotel) fetchRemoteOtelConfig(u *url.URL) (otelMetadata, error) {
	body, err := i.client.Get(u.String(), ipchttp.WithTimeout(httpTO))
	if err != nil {
		return nil, i.log.Error("error fetching remote otel config: %w", err)
	}

	return i.parseResponseFromJSON(body)
}

func (i *inventoryotel) fetchDummyOtelConfig(_ *url.URL) (otelMetadata, error) {
	dummy, err := dummyFS.ReadFile(path.Join("dummy_data", "response.json"))
	if err != nil {
		i.log.Error("Unable to read embedded dummy data:", err)
		return nil, err
	}

	return i.parseResponseFromJSON(dummy)
}

func (i *inventoryotel) fetchOtelAgentMetadata() {
	isEnabled := i.conf.GetBool("otelcollector.enabled")

	if !isEnabled {
		i.log.Infof("OTel Metadata unavailable as OTel collector is disabled")
		i.data = nil

		return
	}
	data, err := i.f.getConfig()
	if err != nil {
		i.log.Error("Unable to fetch fresh inventory metadata: ", err)
		return
	}

	i.data = data
	if i.data == nil {
		i.log.Infof("OTel config returned empty")
		return
	}

	i.data["enabled"] = isEnabled
}

func (i *inventoryotel) refreshMetadata() {
	// Core Agent / agent
	i.fetchOtelAgentMetadata()
}

func (i *inventoryotel) getPayload() marshaler.JSONMarshaler {
	i.m.Lock()
	defer i.m.Unlock()

	i.refreshMetadata()

	// Create a static scrubbed copy of agentMetadata for the payload
	data := copyAndScrub(i.data)

	return &Payload{
		Hostname:  i.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  data,
		UUID:      uuid.GetUUID(),
	}
}

func (i *inventoryotel) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := i.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
