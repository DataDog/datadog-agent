// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventorysigningimpl implements the inventory signing component, to collect package signing keys.
package inventorysigningimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorysigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newInventorySigningProvider),
)

const defaultCollectInterval = 86400 * time.Second

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *signingMetadata `json:"signing_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// TODO implement the split
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories host payload any more, payload is too big for intake")
}

type signingMetadata struct {
	SigningKeys []SigningKey `json:"signing_keys"`
}

type invSigning struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	data     *signingMetadata
	hostname string
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp          inventorysigning.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
}

func newInventorySigningProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	is := &invSigning{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     &signingMetadata{},
	}
	is.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, is.getPayload, "signing.json", time.Duration(defaultCollectInterval))

	return provides{
		Comp:          is,
		Provider:      is.MetadataProvider(),
		FlareProvider: is.FlareProvider(),
	}
}

func (is *invSigning) fillData() {
	if runtime.GOOS == "linux" {
		if platform, err := kernel.Platform(); err == nil {
			switch platform {
			case "debian", "ubuntu":
				is.data.SigningKeys = GetDebianSignatureKeys()
			default: // We are in linux OS, all other distros are redhat based
				is.data.SigningKeys = GetRedhatSignatureKeys()
			}
		}
	}
}

func (is *invSigning) getPayload() marshaler.JSONMarshaler {
	is.fillData()

	return &Payload{
		Hostname:  is.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  is.data,
	}
}
