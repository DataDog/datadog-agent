// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
	// ProcdumpExe is the path to the procdump64 executable
	ProcdumpExe = "C:/procdump/procdump64.exe"
	// ProcdumpZipPath is the path where the procdump zip file is downloaded
	ProcdumpZipPath = "C:/procdump.zip"
	// ProcdumpDownloadURL is the URL to download procdump from Sysinternals
	ProcdumpDownloadURL = "https://download.sysinternals.com/files/Procdump.zip"
)

// SetupProcdump downloads and extracts procdump to the remote host if not already present.
//
// The function downloads procdump directly from Microsoft Sysinternals. This approach
// provides a reliable fallback that doesn't require pre-populating an artifact bucket.
//
// Note: For improved reliability/speed in CI, procdump could be added to the artifact
// bucket (similar to xperf at "windows-products/xperf-5.0.8169.zip") and downloaded
// via host.HostArtifactClient.Get(). The Sysinternals download serves as a fallback.
func SetupProcdump(host *components.RemoteHost) error {
	cmd := fmt.Sprintf(`
		if (-Not (Test-Path -Path '%s')) {
			Invoke-WebRequest -Uri '%s' -OutFile '%s'
			Expand-Archive -Path '%s' -DestinationPath '%s' -Force
		}
	`, ProcdumpPath, ProcdumpDownloadURL, ProcdumpZipPath, ProcdumpZipPath, ProcdumpPath)

	_, err := host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("failed to setup procdump: %w", err)
	}
	return nil
}

// CaptureProcdump captures a full memory dump of a process by PID
//
// The dump file is written to outputDir with the format <processName>_<pid>.dmp
func CaptureProcdump(host *components.RemoteHost, pid int, outputDir string, processName string) (string, error) {
	dumpFileName := fmt.Sprintf("%s_%d.dmp", processName, pid)
	dumpPath := filepath.Join(outputDir, dumpFileName)
	// Use forward slashes for PowerShell compatibility
	dumpPath = strings.ReplaceAll(dumpPath, "\\", "/")

	cmd := fmt.Sprintf(`& "%s" -accepteula -ma %d "%s"`, ProcdumpExe, pid, dumpPath)
	_, err := host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("procdump failed for PID %d: %w", pid, err)
	}
	return dumpPath, nil
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
// This should be called after Wait() returns.
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
