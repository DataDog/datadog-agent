// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func statusCommand() *cobra.Command {
	var debug bool
	var jsonOutput bool

	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Print the installer status",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) error {
			return status(debug, jsonOutput)
		},
	}

	statusCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode")
	statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return statusCmd
}

//go:embed status.tmpl
var statusTmpl []byte

var functions = template.FuncMap{
	"greenText":  color.GreenString,
	"yellowText": color.YellowString,
	"redText":    color.RedString,
	"boldText":   color.New(color.Bold).Sprint,
	"italicText": color.New(color.Italic).Sprint,
	"htmlSafe": func(html string) template.HTML {
		return template.HTML(html)
	},
}

type statusResponse struct {
	Version            string                      `json:"version"`
	Packages           *repository.PackageStates   `json:"packages"`
	ApmInjectionStatus apmInjectionStatus          `json:"apm_injection_status"`
	RemoteConfigState  []*remoteConfigPackageState `json:"remote_config_state"`
}

// apmInjectionStatus contains the instrumentation status of the APM injection.
type apmInjectionStatus struct {
	HostInstrumented   bool `json:"host_instrumented"`
	DockerInstalled    bool `json:"docker_installed"`
	DockerInstrumented bool `json:"docker_instrumented"`
}

func status(debug bool, jsonOutput bool) error {
	tmpl, err := template.New("status").Funcs(functions).Parse(string(statusTmpl))
	if err != nil {
		return fmt.Errorf("error parsing status template: %w", err)
	}

	// Get states & convert to map[string]packageState
	packageStates, err := getState()
	if err != nil {
		return fmt.Errorf("error getting package states: %w", err)
	}

	apmSSIStatus, err := getAPMInjectionStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting APM injection status: %s", err.Error())
	}

	status := statusResponse{
		Version:            version.AgentVersion,
		Packages:           packageStates,
		ApmInjectionStatus: apmSSIStatus,
	}

	if debug {
		// Remote Config status may be confusing for customers, so we only print it in debug mode
		remoteConfigStatus, err := getRCStatus()
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
		status.RemoteConfigState = remoteConfigStatus.PackageStates
	}

	if !jsonOutput {
		err = tmpl.Execute(os.Stdout, status)
		if err != nil {
			return fmt.Errorf("error executing status template: %w", err)
		}
	} else {
		rawResult, err := json.Marshal(status)
		if err != nil {
			return fmt.Errorf("error marshalling status response: %w", err)
		}
		fmt.Println(string(rawResult))
	}
	return nil
}

func getAPMInjectionStatus() (status apmInjectionStatus, err error) {
	// Host is instrumented if the ld.so.preload file contains the apm injector
	ldPreloadContent, err := os.ReadFile("/etc/ld.so.preload")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/ld.so.preload: %w", err)
	}
	if bytes.Contains(ldPreloadContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.HostInstrumented = true
	}

	// Docker is installed if the docker binary is in the PATH
	_, err = exec.LookPath("docker")
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return status, nil
	} else if err != nil {
		return status, fmt.Errorf("could not check if docker is installed: %w", err)
	}
	status.DockerInstalled = true

	// Docker is instrumented if there is the injector runtime in its configuration
	// We're not retrieving the default runtime from the docker daemon as we are not
	// root
	dockerConfigContent, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/docker/daemon.json: %w", err)
	} else if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if bytes.Contains(dockerConfigContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.DockerInstrumented = true
	}

	return status, nil
}

// remoteConfigState is the response to the daemon status route.
// It is technically a json-encoded protobuf message but importing
// the protos in the installer binary is too heavy.
type remoteConfigState struct {
	PackageStates []*remoteConfigPackageState `json:"remote_config_state"`
}

type remoteConfigPackageState struct {
	Package                 string                   `json:"package"`
	StableVersion           string                   `json:"stable_version,omitempty"`
	ExperimentVersion       string                   `json:"experiment_version,omitempty"`
	Task                    *remoteConfigPackageTask `json:"task,omitempty"`
	StableConfigVersion     string                   `json:"stable_config_version,omitempty"`
	ExperimentConfigVersion string                   `json:"experiment_config_version,omitempty"`
}

type remoteConfigPackageTask struct {
	ID    string         `json:"id,omitempty"`
	State int32          `json:"state,omitempty"`
	Error *errorWithCode `json:"error,omitempty"`
}

type errorWithCode struct {
	Code    uint64 `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func getRCStatus() (remoteConfigState, error) {
	var response remoteConfigState

	// The simplest thing here is to call ourselves with the daemon command
	installerBinary, err := os.Executable()
	if err != nil {
		return response, fmt.Errorf("could not get installer binary path: %w", err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := exec.Command(installerBinary, "daemon", "rc-status")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		return response, fmt.Errorf("error running \"datadog-installer daemon rc-status\" (is the daemon running?): %s", stderr.String())
	}

	err = json.Unmarshal(stdout.Bytes(), &response)
	if err != nil {
		return response, fmt.Errorf("error unmarshalling response: %w", err)
	}
	return response, nil
}
