// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package cli implements the compliance check command line interface
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/compliance/aptconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/k8sconfig"
	"github.com/DataDog/datadog-agent/pkg/compliance/types"
	complianceutils "github.com/DataDog/datadog-agent/pkg/compliance/utils"
)

// LoadParams contains parameters for the compliance load command
type LoadParams struct {
	ConfType string
	ProcPid  int
}

// FillLoadFlags fills the load command flags with the provided parameters
func FillLoadFlags(flagSet *pflag.FlagSet, loadArgs *LoadParams) {
	flagSet.IntVarP(&loadArgs.ProcPid, "proc-pid", "", 0, "Process PID for database benchmarks")
}

// RunLoad runs a config load
func RunLoad(_ log.Component, _ config.Component, loadArgs *LoadParams) error {
	hostroot := os.Getenv("HOST_ROOT")
	var resourceType types.ResourceType
	var resource interface{}
	ctx := context.Background()
	switch loadArgs.ConfType {
	case "k8s", "kubernetes":
		resourceType, resource = k8sconfig.LoadConfiguration(ctx, hostroot)
	case "apt":
		resourceType, resource = aptconfig.LoadConfiguration(ctx, hostroot)
	case "db", "database":
		if loadArgs.ProcPid == 0 {
			return errors.New("missing required flag --proc-pid")
		}
		proc, _, rootPath, err := getProcMeta(hostroot, int32(loadArgs.ProcPid))
		if err != nil {
			return err
		}
		var ok bool
		resourceType, resource, ok = dbconfig.LoadConfiguration(ctx, rootPath, proc)
		if !ok {
			return fmt.Errorf("failed to load database config from process %d in %q", loadArgs.ProcPid, rootPath)
		}
	default:
		return fmt.Errorf("unknown config type %q", loadArgs.ConfType)
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
