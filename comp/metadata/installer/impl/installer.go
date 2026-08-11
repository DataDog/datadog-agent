// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installerimpl implements the installer metadata providers interface
package installerimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	installer "github.com/DataDog/datadog-agent/comp/metadata/installer/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	installerstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const (
	flareFileName = "installer.json"

	// fetchTimeout bounds the whole status fetch. The installer daemon answers from
	// memory plus one statfs, so anything slower means it is wedged and we would
	// rather report it unreachable than hold up the metadata runner.
	fetchTimeout = 5 * time.Second
)

// for testing
var fetchInstallerStatus = func(ctx context.Context) (*installerstatus.Response, error) {
	return installerstatus.NewClient(installerstatus.Address()).Status(ctx)
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string                 `json:"hostname"`
	Timestamp int64                  `json:"timestamp"`
	UUID      string                 `json:"uuid"`
	Metadata  map[string]interface{} `json:"installer_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

type inst struct {
	util.InventoryPayload

	log      log.Component
	hostname string
}

// Requires defines the dependencies for the installer metadata component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

// Provides defines the output of the installer metadata component
type Provides struct {
	Comp             installer.Component
	MetadataProvider runnerdef.Provider
	FlareProvider    flaretypes.Provider
	Endpoint         api.AgentEndpointProvider
}

// NewComponent creates a new installer metadata Component
func NewComponent(deps Requires) Provides {
	hname, _ := deps.Hostname.Get(context.Background())
	i := &inst{
		log:      deps.Log,
		hostname: hname,
	}
	i.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, i.getPayload, flareFileName)

	return Provides{
		Comp:             i,
		MetadataProvider: i.MetadataProvider(),
		FlareProvider:    i.FlareProvider(),
		Endpoint:         api.NewAgentEndpointProvider(i.writePayloadAsJSON, "/metadata/installer", "GET"),
	}
}

func (i *inst) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := i.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

// getInstallerMetadata queries the installer daemon's read-only status API.
//
// It never fails: a host without the installer, or with an installer we cannot
// reach, reports `installer_reachable: false` rather than nothing at all. That
// distinction is the point — silence is indistinguishable from a collection bug,
// while an explicit false is a fact about the host.
func (i *inst) getInstallerMetadata() map[string]interface{} {
	metadata := map[string]interface{}{
		"installer_reachable": false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	response, err := fetchInstallerStatus(ctx)
	if err != nil {
		// Debug only: not having an installer daemon is the normal case on a host
		// without remote updates, and it must not produce recurring error logs.
		i.log.Debugf("could not fetch installer status: %s", err)
		return metadata
	}

	metadata["installer_reachable"] = true
	metadata["installer_version"] = response.InstallerVersion
	if response.AvailableDiskSpace != nil {
		metadata["available_disk_space"] = *response.AvailableDiskSpace
	}
	return metadata
}

func (i *inst) getPayload() marshaler.JSONMarshaler {
	return &Payload{
		Hostname:  i.hostname,
		Timestamp: time.Now().UnixNano(),
		UUID:      uuid.GetUUID(),
		Metadata:  i.getInstallerMetadata(),
	}
}
