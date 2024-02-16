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
	"net/http"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newPackageSigningProvider))
}

const defaultCollectInterval = 12 * time.Hour

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *signingMetadata `json:"signing_metadata"`
	UUID      string           `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// TODO implement the split
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split packagesigning payload any more, payload is too big for intake")
}

type signingMetadata struct {
	SigningKeys []signingKey `json:"signing_keys"`
}

type pkgSigning struct {
	util.InventoryPayload

	log        log.Component
	conf       config.Component
	hostname   string
	pkgManager string
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

// Testing purpose
var (
	getPkgManager = pkgUtils.GetPackageManager
	getAPTKeys    = getAPTSignatureKeys
	getYUMKeys    = getYUMSignatureKeys
)

const (
	supportedPkgManager = "apt, yum, dnf, zypper"
)

func newPackageSigningProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	is := &pkgSigning{
		conf:       deps.Config,
		log:        deps.Log,
		hostname:   hname,
		pkgManager: getPkgManager(),
	}
	is.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, is.getPayload, "signing.json")
	is.InventoryPayload.MaxInterval = defaultCollectInterval
	is.InventoryPayload.MinInterval = defaultCollectInterval
	is.InventoryPayload.Enabled = isPackageSigningEnabled(deps.Config, is.log)
	provider := runnerimpl.NewEmptyProvider()
	if runtime.GOOS == "linux" {
		if getPkgManager() != "" {
			// Package signing telemetry is only valid on Linux and DEB/RPM based distros (including SUSE)
			provider = is.MetadataProvider()
		} else {
			is.log.Infof("Package Manager not in [%s], package signing telemetry will not be collected\n", supportedPkgManager)
		}
	}

	return provides{
		Comp:          is,
		Provider:      provider,
		FlareProvider: is.FlareProvider(),
	}
}

func isPackageSigningEnabled(conf config.Reader, logger log.Component) bool {
	if !conf.GetBool("enable_signing_metadata_collection") {
		logger.Debug("Signing metadata collection disabled: linux package signing keys will not be collected nor sent")
		return false
	}
	logger.Debug("Signing metadata collection enabled")
	return true
}

func (is *pkgSigning) getData() []signingKey {

	transport := httputils.CreateHTTPTransport(is.conf)
	client := &http.Client{Transport: transport}

	switch is.pkgManager {
	case "apt":
		return getAPTKeys(client, is.log)
	case "yum", "dnf", "zypper":
		return getYUMKeys(is.pkgManager, client, is.log)
	default: // should not happen, tested above
		is.log.Info("No supported package manager detected, package signing telemetry will not be collected")
	}
	return nil
}

func (is *pkgSigning) getPayload() marshaler.JSONMarshaler {

	return &Payload{
		Hostname:  is.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  &signingMetadata{is.getData()},
		UUID:      uuid.GetUUID(),
	}
}
