// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Creator creates and sends Cluster Agent flares.
type Creator struct {
	cfg          pkgconfigmodel.Reader
	statusComp   status.Component
	diagnoseComp diagnose.Component
	ipcComp      ipc.Component
}

// NewCreator returns a Creator.
func NewCreator(cfg pkgconfigmodel.Reader, statusComp status.Component, diagnoseComp diagnose.Component, ipcComp ipc.Component) *Creator {
	return &Creator{
		cfg:          cfg,
		statusComp:   statusComp,
		diagnoseComp: diagnoseComp,
		ipcComp:      ipcComp,
	}
}

// CreateWithArgs creates a Cluster Agent flare and returns its path.
func (c *Creator) CreateWithArgs(flareArgs flaretypes.FlareArgs, _ time.Duration, _ error, _ []byte) (string, error) {
	return createIdentifiedDCAArchive(c.cfg, flareArgs, c.statusComp, c.diagnoseComp, c.ipcComp)
}

// Send sends the flare to Datadog support.
func (c *Creator) Send(flarePath string, caseID string, email string, source flarehelpers.FlareSource) (string, error) {
	return sendFlareFunc(c.cfg, flarePath, caseID, email, c.cfg.GetString("api_key"), configUtils.GetInfraEndpoint(c.cfg), source)
}

func createIdentifiedDCAArchive(cfg pkgconfigmodel.Reader, flareArgs flaretypes.FlareArgs, statusComp status.Component, diagnoseComp diagnose.Component, ipcComp ipc.Component) (string, error) {
	logFile := defaultpaths.GetDefaultDCALogFile()
	if cfg.IsConfigured("log_file") {
		logFile = cfg.GetString("log_file")
	}

	filePath, err := createDCAArchiveFunc(false, defaultpaths.GetDistPath(), logFile, nil, flareArgs, statusComp, diagnoseComp, ipcComp)
	if err != nil {
		return "", err
	}
	if clusterName, namespace, podName, identityErr := getClusterAgentIdentityFunc(); identityErr != nil {
		log.Infof("Could not add Cluster Agent resource identity to flare archive name: %v", identityErr)
	} else {
		filePath = renameClusterAgentFlareArchive(filePath, clusterName, namespace, podName)
	}
	return filePath, nil
}
