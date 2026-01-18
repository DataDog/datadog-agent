// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

// RemoteHost represents a remote host
type RemoteHost struct {
	remote.HostOutput

	*client.Host
	context common.Context
}

var _ common.Initializable = (*RemoteHost)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (h *RemoteHost) Init(ctx common.Context) (err error) {
	h.context = ctx
	h.Host, err = client.NewHost(ctx, h.HostOutput)
	return err
}

// DownloadAgentLogs downloads the agent logs from the remote host
func (h *RemoteHost) DownloadAgentLogs(localPath string) error {
	agentLogsPath := "/var/log/datadog/agent.log"
	if h.OSFamily == osComp.WindowsFamily {
		agentLogsPath = "C:/ProgramData/Datadog/Logs/agent.log"
	}
	return h.Host.GetFile(agentLogsPath, localPath)
}
