// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packagesigningimpl implements the inventory signing component, to collect package signing keys.
package packagesigningimpl

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
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newPackageSigningProvider),
)

const defaultCollectInterval = 12 * time.Hour

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

type pkgSigning struct {
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

	Comp          packagesigning.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
}

func newPackageSigningProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	is := &pkgSigning{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     &signingMetadata{},
	}
	is.InventoryPayload.SetIntervals(defaultCollectInterval, defaultCollectInterval)
	is.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, is.getPayload, "signing.json")
	provider := runnerimpl.NewEmptyProvider()
	if runtime.GOOS == "linux" && getPackageManager() != "" {
		// Package signing telemetry is only valid on Linux and DEB/RPM based distros (including SUSE)
		provider = is.MetadataProvider()
	}

	return provides{
		Comp:          is,
		Provider:      provider,
		FlareProvider: is.FlareProvider(),
	}
}

func (is *pkgSigning) fillData() {
	pkgManager := getPackageManager()
	switch pkgManager {
	case "apt":
		is.data.SigningKeys = getAPTSignatureKeys()
	case "yum", "dnf", "zypper":
		is.data.SigningKeys = getYUMSignatureKeys(pkgManager)
	default: // should not happen, tested above
		is.log.Info("No supported package manager detected, package signing telemetry will not be collected")
	}
}

func (is *pkgSigning) getPayload() marshaler.JSONMarshaler {
	is.fillData()

	return &Payload{
		Hostname:  is.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  is.data,
	}
}

// GetLinuxPackageSigningPolicy returns the global GPG signing policy of the host
func GetLinuxPackageSigningPolicy() bool {
	if runtime.GOOS == "linux" {
		pkgManager := getPackageManager()
		switch pkgManager {
		case "apt":
			return getNoDebsig()
		case "yum", "dnf", "zypper":
			return getMainGPGCheck(pkgManager)
		default: // should not happen, tested above
			return false
		}
	}
	return true
}
