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
	"strings"
	"time"

	"go.uber.org/fx"

	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
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
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
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
	Endpoint      apihelper.AgentEndpointProvider
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
	var provider runnerimpl.Provider
	if is.InventoryPayload.Enabled {
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
		Endpoint:      apihelper.NewAgentEndpointProvider(is.writePayloadAsJSON, "/metadata/package-signing", "GET"),
	}
}

func isPackageSigningEnabled(conf config.Reader, logger log.Component) bool {
	installTool, err := installinfo.Get(conf)
	if err != nil {
		logger.Debugf("Failed to get install_info file information: %v", err)
		return false
	}
	isInConfigurationFile := conf.GetBool("enable_signing_metadata_collection")
	isEnabled := isInConfigurationFile && runtime.GOOS == "linux" && isAllowedInstallationTool(installTool.Tool)
	if !isEnabled {
		logger.Debugf("Package-signing metadata collection disabled: config %t, OS %s, install tool %s", isInConfigurationFile, runtime.GOOS, installTool.Tool)
		logger.Debug("Package-signing metadata must be enabled in datadog.yaml, and running on a non-containerized Linux system to collect data")
	} else {
		logger.Debug("Package-signing metadata collection enabled")
	}
	return isEnabled
}

// isAllowedInstallationTool returns false if we detect a container-like installation method
func isAllowedInstallationTool(installTool string) bool {
	forbiddenMethods := []string{"helm", "docker", "operator", "kube"}
	for _, method := range forbiddenMethods {
		if strings.Contains(installTool, method) {
			return false
		}
	}
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

func (is *pkgSigning) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := is.GetAsJSON()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
