// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
	t.Logf("Running %s command. Waiting for %s container to be running", cfg.command(), cfg.Name())
	var err error
	var ctx context.Context
	for i := 0; i < cfg.Retries(); i++ {
		t.Helper()
		// Ensuring no previous instances exists.
		killPreviousInstances(cfg)

		// attempt to start the container/s
		ctx, err = run(t, cfg)
		if err != nil {
			t.Logf("could not start %s: %v", cfg.Name(), err)
			//this iteration failed, retry
			continue
		}

		//check container logs for successful start
		if err = checkReadiness(ctx, cfg); err == nil {
			// target container/s started successfully, we can stop the retries loop and finish here
			t.Logf("%s command succeeded. %s container is running", cfg.command(), cfg.Name())
			return nil
		}
		t.Logf("[Attempt #%v] failed to start %s server: %v", i+1, cfg.Name(), err)
		cfg.PatternScanner().PrintLogs(t)
		time.Sleep(5 * time.Second)
	}
	return err
}

// we do best-effort to kill previous instances, hence ignoring any errors
func killPreviousInstances(cfg LifecycleConfig) {
	// Ensuring the following command won't block forever
	timedContext, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()
	args := cfg.commandArgs(kill)

	// Ensuring no previous instances exists.
	c := exec.CommandContext(timedContext, cfg.command(), args...)
	c.Env = append(c.Env, cfg.Env()...)

	// run synchronously to ensure all instances are killed
	_ = c.Run()
}

func run(t testing.TB, cfg LifecycleConfig) (context.Context, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	args := cfg.commandArgs(start)

	//prepare the command
	cmd := exec.CommandContext(ctx, cfg.command(), args...)
	cmd.Env = append(cmd.Env, cfg.Env()...)
	cmd.Stdout = cfg.PatternScanner()
	cmd.Stderr = cfg.PatternScanner()

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

func checkReadiness(ctx context.Context, cfg LifecycleConfig) error {
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("failed to start the container %s due to: %w", cfg.Name(), err)
			}
		case <-cfg.PatternScanner().DoneChan:
			return nil
		case <-time.After(cfg.Timeout()):
			return fmt.Errorf("failed to start the container %s, reached timeout of %v", cfg.Name(), cfg.Timeout())
		}
	}
}
