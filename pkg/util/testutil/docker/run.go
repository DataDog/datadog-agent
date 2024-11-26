// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package docker

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// Run starts the container/s and ensures their successful invocation
// LifecycleConfig is an interface that abstracts the configuration of the container/s
// Use NewRunConfig to run a single docker container or NewComposeConfig to spin docker-compose
// This method is using testing.TB interface to handle cleanup and logging during UTs execution
func Run(t testing.TB, cfg LifecycleConfig) error {
	var err error
	var ctx context.Context
	for i := 0; i < cfg.Retries(); i++ {
		t.Helper()
		// Ensuring no previous instances exists.
		killPreviousInstances(cfg)

		scanner := NewScanner(cfg.LogPattern(), make(chan struct{}, 1))
		// attempt to start the container/s
		ctx, err = run(t, cfg, scanner)
		if err != nil {
			t.Logf("could not start %s: %v", cfg.Name(), err)
			//this iteration failed, retry
			continue
		}

		//check container logs for successful start
		if err = checkReadiness(ctx, cfg, scanner); err == nil {
			// target container/s started successfully, we can stop the retries loop and finish here
			return nil
		}
		scanner.PrintLogs(t)
		t.Logf("failed to start %s server, retrying: %v", cfg.Name(), err)
		time.Sleep(5 * time.Second)
	}
	return err
}

// isStart is true if command is to start the container, false if to stop the container
func buildCommandArgs(cfg LifecycleConfig, isStart bool) []string {
	var args []string
	switch concreteCfg := cfg.(type) {
	case runConfig:
		if isStart {
			args = []string{string(runCommand), "--rm"}

			// Add mounts
			for hostPath, containerPath := range concreteCfg.Mounts {
				args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
			}

			// Pass environment variables to the container as docker args
			for _, env := range concreteCfg.Env() {
				args = append(args, "-e", env)
			}

			//append container name and container image name
			args = append(args, "--name", concreteCfg.Name(), concreteCfg.ImageName)

			//provide main binary and binary arguments to run inside the docker container
			args = append(args, concreteCfg.Binary)
			args = append(args, concreteCfg.BinaryArgs...)
		} else {
			args = []string{string(removeCommand), "-f", concreteCfg.Name(), "--volumes"}
		}
	case composeConfig:
		if isStart {
			args = []string{string(composeCommand), "-f", concreteCfg.File, "up", "--remove-orphans", "-V"}
		} else {
			args = []string{string(composeCommand), "-f", concreteCfg.File, "down", "--remove-orphans", "--volumes"}
		}
	}
	return args
}

// we try best effort to kill previous instances, hence ignoring any errors
func killPreviousInstances(cfg LifecycleConfig) {
	// Ensuring the following command won't block forever
	timedContext, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()
	args := buildCommandArgs(cfg, false)

	// Ensuring no previous instances exists.
	c := exec.CommandContext(timedContext, "docker", args...)
	c.Env = append(c.Env, cfg.Env()...)

	// run synchronously to ensure all instances are killed
	_ = c.Run()
}

func run(t testing.TB, cfg LifecycleConfig, scanner *PatternScanner) (context.Context, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	args := buildCommandArgs(cfg, true)

	//prepare the command
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(cmd.Env, cfg.Env()...)
	cmd.Stdout = scanner
	cmd.Stderr = scanner

	// run asynchronously and don't wait for the command to finish
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	//register cleanup function to kill the instances upon finishing the test
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		killPreviousInstances(cfg)
	})

	return ctx, nil
}

func checkReadiness(ctx context.Context, cfg LifecycleConfig, scanner *PatternScanner) error {
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("failed to start the container %s due to: %w", cfg.Name(), err)
			}
		case <-scanner.DoneChan:
			return nil
		case <-time.After(cfg.Timeout()):
			return fmt.Errorf("failed to start the container %s, after %v timeout", cfg.Name(), cfg.Timeout())
		}
	}
}
