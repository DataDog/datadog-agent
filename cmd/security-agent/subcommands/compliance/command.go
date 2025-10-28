// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements compliance related subcommands
package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/compliance/aptconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/types"
	complianceutils "github.com/DataDog/datadog-agent/pkg/compliance/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns the compliance commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	complianceCmd.AddCommand(check.SecurityAgentCommands(globalParams)...)
	complianceCmd.AddCommand(complianceLoadCommand(globalParams))

	return []*cobra.Command{complianceCmd}
}

type loadCliParams struct {
	*command.GlobalParams
	confType string
	procPid  int
}

func complianceLoadCommand(globalParams *command.GlobalParams) *cobra.Command {
	loadArgs := &loadCliParams{
		GlobalParams: globalParams,
	}

	loadCmd := &cobra.Command{
		Use:   "load <conf-type>",
		Short: "Load compliance config",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			loadArgs.confType = args[0]
			return fxutil.OneShot(loadRun,
				fx.Supply(loadArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
				logscompressionfx.Module(),
			)
		},
	}

	loadCmd.Flags().IntVarP(&loadArgs.procPid, "proc-pid", "", 0, "Process PID for database benchmarks")
	return loadCmd
}

func loadRun(_ log.Component, _ config.Component, loadArgs *loadCliParams) error {
	hostroot := os.Getenv("HOST_ROOT")
	var resourceType types.ResourceType
	var resource interface{}
	ctx := context.Background()
	switch loadArgs.confType {
	case "k8s", "kubernetes":
		resourceType, resource = k8sconfig.LoadConfiguration(ctx, hostroot)
	case "apt":
		resourceType, resource = aptconfig.LoadConfiguration(ctx, hostroot)
	case "db", "database":
		if loadArgs.procPid == 0 {
			return fmt.Errorf("missing required flag --proc-pid")
		}
		proc, _, rootPath, err := getProcMeta(hostroot, int32(loadArgs.procPid))
		if err != nil {
			return err
		}
		var ok bool
		resourceType, resource, ok = dbconfig.LoadConfiguration(ctx, rootPath, proc)
		if !ok {
			return fmt.Errorf("failed to load database config from process %d in %q", loadArgs.procPid, rootPath)
		}
	default:
		return fmt.Errorf("unknown config type %q", loadArgs.confType)
	}
	resourceData, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s config: %v", resourceType, err)
	}
	fmt.Fprintf(os.Stderr, "Loaded config with resource type %q\n", resourceType)
	fmt.Println(string(resourceData))
	return nil
}

func getProcMeta(hostroot string, pid int32) (*process.Process, complianceutils.ContainerID, string, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get process with pid %d: %v", pid, err)
	}
	containerID, _ := complianceutils.GetProcessContainerID(proc.Pid)
	var rootPath string
	if containerID != "" {
		rootPath, err = complianceutils.GetContainerOverlayPath(proc.Pid)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get container overlay path for process %d: %v", pid, err)
		}
	} else {
		rootPath = "/"
	}
	return proc, containerID, filepath.Join(hostroot, rootPath), nil
}
