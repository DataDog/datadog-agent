// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
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

// AddUserToAgentGroup adds the current user of the ssh connection to the `dd-agent` group.
// Useful to access the logs files since the directory `/var/log/datadog` is restricted to the `dd-agent` user and group.
func (h *RemoteHost) AddUserToAgentGroup() error {
	cmd := fmt.Sprintf("sudo usermod -aG dd-agent %s", h.Username)
	_, err := h.Execute(cmd)
	if err != nil {
		return fmt.Errorf("Unable to add ssh user to `dd-agent` group: %v", err)
	}
	// Reconnect for the group membership to be updated
	if err := h.Reconnect(); err != nil {
		return fmt.Errorf("Unable to reconnect to the host: %v", err)
	}
	return nil
}
