// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// ProcdumpPath is the directory where procdump is installed
	ProcdumpPath = "C:/procdump"
	// ProcdumpExe is the path to the procdump executable
	ProcdumpExe = "C:/procdump/procdump.exe"
	// ProcdumpZipPath is the path where the procdump zip file is downloaded
	ProcdumpZipPath = "C:/procdump.zip"
	// ProcdumpDownloadURL is the URL to download procdump from Sysinternals
	ProcdumpDownloadURL = "https://download.sysinternals.com/files/Procdump.zip"
	// ProcdumpsPath is the directory where procdump captures are stored.
	ProcdumpsPath = "C:/procdumps"
)

// SetupProcdump downloads and extracts procdump to the remote host if not already present.
func SetupProcdump(host *components.RemoteHost) error {
	err := host.HostArtifactClient.Get("windows-products/Procdump.zip", ProcdumpZipPath)
	if err != nil {
		return fmt.Errorf("failed to download procdump: %w", err)
	}

	_, err = host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { Expand-Archive -Path '%s' -DestinationPath '%s' }`, ProcdumpPath, ProcdumpZipPath, ProcdumpPath))
	if err != nil {
		return fmt.Errorf("failed to setup procdump: %w", err)
	}

	// Create the procdump output directory (separate from WER dumps)
	_, err = host.Execute(fmt.Sprintf(`New-Item -ItemType Directory -Path '%s' -Force`, ProcdumpsPath))
	if err != nil {
		return fmt.Errorf("failed to create procdump output directory: %w", err)
	}

	return nil
}

// CaptureProcdump captures a full memory dump of a process by PID
func CaptureProcdump(host *components.RemoteHost, pid int, outputDir string, processName string) (string, error) {
	dumpFileName := fmt.Sprintf("%s.%d.dmp", processName, pid)
	dumpPath := filepath.Join(outputDir, dumpFileName)
	// Use forward slashes for PowerShell compatibility
	dumpPath = strings.ReplaceAll(dumpPath, "\\", "/")

	cmd := fmt.Sprintf(`& "%s" -accepteula -ma %d "%s"`, ProcdumpExe, pid, dumpPath)
	output, err := host.Execute(cmd)

	// Procdump returns exit code 1 when "Dump count reached", which is success.
	// Check both output and error message to determine if the dump was captured.
	// When host.Execute returns an error, the command output may be embedded in
	// the error message rather than the output string.
	successIndicator := "Dump 1 complete"
	if strings.Contains(output, successIndicator) {
		// Dump was successful, ignore exit code
		return dumpPath, nil
	}
	if err != nil && strings.Contains(err.Error(), successIndicator) {
		// Dump was successful but output was in error message, ignore exit code
		return dumpPath, nil
	}

	// If we don't see success in output or error and there was an error, report it
	if err != nil {
		return "", fmt.Errorf("procdump failed for PID %d: %w\nOutput: %s", pid, err, output)
	}

	// No error but also no success message - unexpected
	return "", fmt.Errorf("procdump did not capture dump for PID %d, output: %s", pid, output)
}

// StartupDumpCollector monitors a Windows service and captures a memory dump
// during the startup phase. It polls for the service to enter the "StartPending"
// state, waits a configurable delay, then captures a dump using procdump.
type StartupDumpCollector struct {
	host        *components.RemoteHost
	serviceName string
	outputDir   string
	delay       time.Duration // delay after StartPending before capturing

	dumpPaths []string
	err       error
	done      chan struct{}
}

// NewStartupDumpCollector creates a new StartupDumpCollector
//
// Parameters:
//   - host: the remote Windows host to monitor
//   - serviceName: the Windows service name to monitor (e.g., "datadogagent")
//   - outputDir: directory on the remote host where dumps will be written
func NewStartupDumpCollector(host *components.RemoteHost, serviceName, outputDir string) *StartupDumpCollector {
	return &StartupDumpCollector{
		host:        host,
		serviceName: serviceName,
		outputDir:   outputDir,
		delay:       10 * time.Second,
		done:        make(chan struct{}),
	}
}

// WithDelay sets the delay between detecting StartPending and capturing the dump
func (c *StartupDumpCollector) WithDelay(d time.Duration) *StartupDumpCollector {
	c.delay = d
	return c
}

// Start begins monitoring the service in a background goroutine.
// The goroutine will:
// 1. Poll the service status every second
// 2. When "StartPending" is detected, wait for the configured delay
// 3. Capture a memory dump of the service process
// 4. Exit after capturing one dump or when the context is cancelled
func (c *StartupDumpCollector) Start(ctx context.Context) {
	go func() {
		defer close(c.done)

		// Poll for StartPending state
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := GetServiceStatus(c.host, c.serviceName)
				if err != nil {
					// Service may not exist yet, continue polling
					continue
				}

				if strings.Contains(status, "StartPending") {
					// Found StartPending, wait the configured delay then capture
					select {
					case <-ctx.Done():
						return
					case <-time.After(c.delay):
					}

					// Get the service PID and capture dump
					pid, err := GetServicePID(c.host, c.serviceName)
					if err != nil {
						c.err = fmt.Errorf("failed to get service PID for %s: %w", c.serviceName, err)
						return
					}

					if pid == 0 {
						c.err = fmt.Errorf("service %s has PID 0 (not running)", c.serviceName)
						return
					}

					dumpPath, err := CaptureProcdump(c.host, pid, c.outputDir, c.serviceName)
					if err != nil {
						c.err = err
						return
					}
					c.dumpPaths = append(c.dumpPaths, dumpPath)
					return // Done - captured one dump
				}
			}
		}
	}()
}

// Wait blocks until the collector goroutine is done or the context was cancelled
func (c *StartupDumpCollector) Wait() {
	<-c.done
}

// Results returns the collected dump paths and any error that occurred.
func (c *StartupDumpCollector) Results() ([]string, error) {
	return c.dumpPaths, c.err
}

// DumpPaths returns the paths to collected dump files on the remote host
func (c *StartupDumpCollector) DumpPaths() []string {
	return c.dumpPaths
}

// Error returns any error that occurred during collection
func (c *StartupDumpCollector) Error() error {
	return c.err
}
