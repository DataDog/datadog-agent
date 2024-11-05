// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package net

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	discoverymodel "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	nppayload "github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// SysProbeUtilGetter is a function that returns a SysProbeUtil for the given path
// The standard implementation is GetRemoteSysProbeUtil
type SysProbeUtilGetter func(string) (SysProbeUtil, error)

// SysProbeUtil fetches info from the SysProbe running remotely
type SysProbeUtil interface {
	GetConnections(clientID string) (*model.Connections, error)
	GetStats() (map[string]interface{}, error)
	GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error)
	Register(clientID string) error
	GetNetworkID() (string, error)
	GetTelemetry() ([]byte, error)
	GetConnTrackCached() ([]byte, error)
	GetConnTrackHost() ([]byte, error)
	GetBTFLoaderInfo() ([]byte, error)
	DetectLanguage(pids []int32) ([]languagemodels.Language, error)
	GetPprof(path string) ([]byte, error)
	GetDiscoveryServices() (*discoverymodel.ServicesResponse, error)
	GetCheck(module sysconfigtypes.ModuleName) (interface{}, error)
	GetPing(clientID string, host string, count int, interval time.Duration, timeout time.Duration) ([]byte, error)
	GetTraceroute(clientID string, host string, port uint16, protocol nppayload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error)
}
