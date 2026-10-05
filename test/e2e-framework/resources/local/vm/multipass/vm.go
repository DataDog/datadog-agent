// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localmultipass

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	resourceslocal "github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const defaultUser = "ubuntu"

// cloudInitScript builds a subshell that pipes cloud-init YAML to cmd via heredoc.
// The pipe must be on the same line as the heredoc opener, so the result is
// wrapped in a subshell that can be chained with &&.
func cloudInitScript(content, cmd string) string {
	return fmt.Sprintf("(cat << 'CLOUDEOF' | %s\n%s\nCLOUDEOF\n)", cmd, strings.TrimSpace(content))
}

// multipassInfo is the relevant subset of `multipass info --format json` output.
type multipassInfo struct {
	Info map[string]struct {
		IPv4  []string `json:"ipv4"`
		State string   `json:"state"`
	} `json:"info"`
}

// VMArgs holds configuration for a multipass VM instance.
type VMArgs struct {
	CPUs   string
	Memory string
	Disk   string

	PulumiResourceOptions []pulumi.ResourceOption
}

// NewInstance launches a multipass Ubuntu LTS VM and returns its SSH connection details.
// The public key at e.DefaultPublicKeyPath() (Pulumi config key "local/defaultPublicKeyPath")
// is injected via cloud-init piped to stdin — no files are written to disk.
func NewInstance(e resourceslocal.Environment, name string, args *VMArgs, opts ...pulumi.ResourceOption) (address pulumi.StringOutput, user string, port int, err error) {
	pubKeyBytes, err := os.ReadFile(e.DefaultPublicKeyPath())
	if err != nil {
		return pulumi.StringOutput{}, "", -1, fmt.Errorf("reading SSH public key from %s: %w", e.DefaultPublicKeyPath(), err)
	}

	cloudInit := fmt.Sprintf(`#cloud-config
users:
  - default
ssh_authorized_keys:
  - %s`, strings.TrimSpace(string(pubKeyBytes)))

	launchCmd := fmt.Sprintf("multipass launch --name %s --cpus %s --memory %s --disk %s --cloud-init - lts",
		name, args.CPUs, args.Memory, args.Disk)

	runner := command.NewLocalRunner(&e, command.LocalRunnerArgs{OSCommand: command.NewLocalOSCommand()})

	// Step 1: fetch current VM state. Returns '{}' when the VM does not exist.
	stateResource, err := runner.Command("multipass-state-"+name, &command.LocalArgs{
		Args: command.Args{
			Create:   pulumi.Sprintf("multipass info --format json %s || echo '{}'", name),
			Triggers: pulumi.Array{},
		},
		LocalAssetPaths: pulumi.StringArray{},
		LocalDir:        pulumi.String("."),
	}, opts...)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	// Step 2: parse the state JSON in Go and decide what shell command to run.
	actionStr := stateResource.StdoutOutput().ApplyT(func(s string) (string, error) {
		var info multipassInfo
		if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &info); err != nil {
			return "", fmt.Errorf("parsing multipass state JSON: %w", err)
		}
		vm, ok := info.Info[name]
		if !ok {
			return cloudInitScript(cloudInit, launchCmd), nil
		}
		switch vm.State {
		case "Running":
			return "true", nil // no-op; local.Command requires a non-empty Create string
		case "Stopped", "Suspended":
			return fmt.Sprintf("multipass start %s", name), nil
		case "Deleted":
			return fmt.Sprintf("multipass delete --purge %s && %s", name, cloudInitScript(cloudInit, launchCmd)), nil
		default:
			return cloudInitScript(cloudInit, launchCmd), nil
		}
	}).(pulumi.StringOutput)

	// Step 3: execute the action (launch / start / no-op).
	actionResource, err := runner.Command("multipass-action-"+name, &command.LocalArgs{
		Args: command.Args{
			Create:   actionStr,
			Triggers: pulumi.Array{},
		},
		LocalAssetPaths: pulumi.StringArray{},
		LocalDir:        pulumi.String("."),
	}, utils.MergeOptions(opts, utils.PulumiDependsOn(stateResource))...)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	// Step 4: fetch the final VM info JSON. This resource also owns the delete.
	vm, err := runner.Command("multipass-vm-"+name, &command.LocalArgs{
		Args: command.Args{
			Create:   pulumi.Sprintf("multipass info --format json %s", name),
			Delete:   pulumi.Sprintf("multipass delete --purge %s", name),
			Triggers: pulumi.Array{},
		},
		LocalAssetPaths: pulumi.StringArray{},
		LocalDir:        pulumi.String("."),
	}, utils.MergeOptions(opts, utils.PulumiDependsOn(actionResource))...)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	// Parse the info JSON in Go to extract the IPv4 address.
	address = vm.StdoutOutput().ApplyT(func(s string) (string, error) {
		var info multipassInfo
		if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &info); err != nil {
			return "", fmt.Errorf("parsing multipass info JSON: %w", err)
		}
		vmInfo, ok := info.Info[name]
		if !ok || len(vmInfo.IPv4) == 0 {
			return "", fmt.Errorf("no IPv4 address found for VM %s", name)
		}
		return vmInfo.IPv4[0], nil
	}).(pulumi.StringOutput)

	return address, defaultUser, 22, nil
}
