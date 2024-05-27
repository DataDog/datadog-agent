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
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	iointerface "github.com/DataDog/datadog-agent/comp/metadata/inventoryotel"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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

	conf      config.Component
	log       log.Component
	m         sync.Mutex
	data      otelMetadata
	hostname  string
	authToken authtoken.Component
	f         *freshConfig
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	AuthToken  authtoken.Component
}

type provides struct {
	fx.Out

	Comp                 iointerface.Component
	Provider             runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
}

func newInventoryOtelProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	i := &inventoryotel{
		conf:      deps.Config,
		log:       deps.Log,
		hostname:  hname,
		data:      make(otelMetadata),
		authToken: deps.AuthToken,
	}

	getter := i.fetchRemoteOtelConfig
	if i.conf.GetBool("otel.submit_dummy_inventories_metadata") {
		getter = i.fetchDummyOtelConfig
	}

	var err error
	i.f, err = newFreshConfig(deps.Config.GetString("otel.extension_url"), getter)
	if err != nil {
		// panic?
		return provides{}
	}

	i.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, i.getPayload, "otel.json")

	if i.Enabled {

		// TODO: if there's an update on the OTel side we currently will not be
		//       notified. Is this a problem? Runtime changes are expected to be
		//       triggered by FA, so maybe this is OK.
		//
		// We want to be notified when the configuration is updated
		deps.Config.OnUpdate(func(_ string, _, _ any) { i.Refresh() })
	}

	return provides{
		Comp:                 i,
		Provider:             i.MetadataProvider(),
		FlareProvider:        i.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(i),
	}
}

func scrub(s string) string {
	// Errors come from internal use of a Reader interface. Since we are reading from a buffer, no errors
	// are possible.
	scrubString, _ := scrubber.ScrubString(s)
	return scrubString
}

func (i *inventoryotel) parseResponseFromJSON(body []byte) (otelMetadata, error) {
	var conf interface{}

	err := json.Unmarshal(body, &conf)
	if err != nil {
		i.log.Errorf("Error unmarshaling the response:", err)
		return nil, err
	}

	return conf.(otelMetadata), nil
}

func (i *inventoryotel) fetchRemoteOtelConfig(u *url.URL) (otelMetadata, error) {
	// Create a Bearer string by appending string access token
	var bearer = "Bearer " + i.authToken.Get()

	// Create a new request using http
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		i.log.Errorf("Error building request: ", err)
		return nil, err
	}

	// add authorization header to the req
	req.Header.Add("Authorization", bearer)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		i.log.Errorf("Error on response: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		i.log.Errorf("Error while reading the response bytes:", err)
		return nil, err
	}

	return i.parseResponseFromJSON(body)

}

func (i *inventoryotel) fetchDummyOtelConfig(_ *url.URL) (otelMetadata, error) {
	dummy, err := dummyFS.ReadFile(path.Join("dummy_data", "response.json"))
	if err != nil {
		i.log.Errorf("Unable to read embedded dummy data:", err)
		return nil, err
	}

	return i.parseResponseFromJSON(dummy)
}

func (i *inventoryotel) fetchOtelAgentMetadata() {
	isEnabled := i.conf.GetBool("otel.enabled")

	if !isEnabled {
		i.log.Infof("OTel Metadata unavailable as OTel collector is disabled")
		i.data = nil

		return
	}
	data, err := i.f.getConfig()
	if err != nil {
		i.log.Errorf("Unable to fetch fresh inventory metadata: ", err)
	}

	i.data = data
	if i.data == nil {
		i.log.Infof("OTel config returned empty")
		return
	}

	i.data["otel_enabled"] = isEnabled

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

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (i *inventoryotel) Get() otelMetadata {
	i.m.Lock()
	defer i.m.Unlock()

	data := otelMetadata{}
	for k, v := range i.data {
		data[k] = v
	}
	return data
}
