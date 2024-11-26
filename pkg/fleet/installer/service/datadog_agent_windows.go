// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/winregistry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"path"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	datadogAgent = "datadog-agent"
)

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, args []string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		// Don't log error here, or it will appear twice in the output
		// since installerImpl.Install will also print the error.
		span.Finish(tracer.WithError(err))
	}()
	// Make sure there are no Agent already installed
	_ = removeAgentIfInstalled(ctx)
	err = installAgentPackage("stable", args)
	return err
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to start agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()

	err = removeAgentIfInstalled(ctx)
	if err != nil {
		return err
	}

	err = installAgentPackage("experiment", nil)
	if err != nil {
		// experiment failed, expect stop-experiment to restore the stable Agent
		return err
	}
	return nil
}

// StopAgentExperiment stops the agent experiment, i.e. removes/uninstalls it.
func StopAgentExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to stop agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()

	err = removeAgentIfInstalled(ctx)
	if err != nil {
		return err
	}

	err = installAgentPackage("stable", nil)
	if err != nil {
		// if we cannot restore the stable Agent, the system is left without an Agent
		return err
	}

	return nil
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(_ context.Context) error {
	// noop
	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) (err error) {
	// Don't return an error if the Agent is already not installed.
	// returning an error here will prevent the package from being removed
	// from the local repository.
	return removeAgentIfInstalled(ctx)
}

func installAgentPackage(target string, args []string) error {
	// Lookup Agent user stored in registry by the Installer MSI
	// and pass it to the Agent MSI
	agentUser, err := winregistry.GetAgentUserName()
	if err != nil {
		return fmt.Errorf("failed to get Agent user: %w", err)
	}

	tempDir, err := os.MkdirTemp(paths.RootTmpDir, "datadog-agent")
	if err != nil {
		return err
	}
	logFile := path.Join(tempDir, "msi.log")

	cmd, err := msi.Cmd(
		msi.Install(),
		msi.WithMsiFromPackagePath(target, datadogAgent),
		msi.WithDdAgentUserName(agentUser),
		msi.WithAdditionalArgs(args),
		msi.WithLogFile(path.Join(tempDir, "msi.log")),
	)
	var output []byte
	if err == nil {
		output, err = cmd.Run()
	}
	if err != nil {
		return fmt.Errorf("failed to install Agent %s: %w\nLog file located at: %s\n%s\n", target, err, logFile, string(output))
	}
	return nil
}

func removeAgentIfInstalled(ctx context.Context) (err error) {
	if msi.IsProductInstalled("Datadog Agent") {
		span, _ := tracer.StartSpanFromContext(ctx, "remove_agent")
		defer func() {
			if err != nil {
				// removal failed, this should rarely happen.
				// Rollback might have restored the Agent, but we can't be sure.
				log.Errorf("Failed to remove agent: %s", err)
			}
			span.Finish(tracer.WithError(err))
		}()
		err := msi.RemoveProduct("Datadog Agent")
		if err != nil {
			return err
		}
	} else {
		log.Debugf("Agent not installed")
	}
	return nil
}
