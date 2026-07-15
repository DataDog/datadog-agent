// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const ladingFixtureName = "lading/generated"

var ladingBinaryFlag = flag.String("lading-binary", "", "path to a Lading release binary with OpenMetrics HTTP body support")

type ladingServer struct {
	cmd      *exec.Cmd
	endpoint string
	tempDir  string
	output   bytes.Buffer
}

func findLadingBinary() (string, error) {
	if *ladingBinaryFlag != "" {
		return *ladingBinaryFlag, nil
	}
	if configured := os.Getenv("LADING_BINARY"); configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(home, "dd", "lading", "target", "release", "lading")
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
		return candidate, nil
	}
	return "", fmt.Errorf("Lading release binary not found at %s; pass -lading-binary or set LADING_BINARY", candidate)
}

func reserveLocalAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return address, nil
}

func newLadingServer() (*ladingServer, error) {
	binary, err := findLadingBinary()
	if err != nil {
		return nil, err
	}
	address, err := reserveLocalAddress()
	if err != nil {
		return nil, fmt.Errorf("reserve Lading address: %w", err)
	}
	tempDir, err := os.MkdirTemp("", "openmetrics-differential-lading-")
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(tempDir, "lading.yaml")
	config := fmt.Sprintf(`blackhole:
  - http:
      binding_addr: %q
      concurrent_requests_max: 32
      body_variant:
        openmetrics:
          metric_name_prefix: "diff_lading"
          include_help: true
          include_type: true
          counters: {count: 8}
          gauges: {count: 8}
          histograms: {count: 4, buckets: ["0.1", "0.5", "1", "5"]}
          summaries: {count: 4, quantiles: ["0.5", "0.9", "0.99"]}
          labels:
            services: ["checkout", "catalog", "payments"]
            regions: ["us-east-1", "eu-central-1"]
            methods: ["GET", "POST"]
            status_classes: ["2xx", "5xx"]
            consumers: ["consumer-00", "consumer-01"]
            route_count: 8
`, address)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	server := &ladingServer{endpoint: "http://" + address + "/metrics", tempDir: tempDir}
	server.cmd = exec.Command(binary,
		"--config-path", configPath,
		"--capture-path", filepath.Join(tempDir, "capture.jsonl"),
		"--no-target",
		"--experiment-duration-infinite",
		"--warmup-duration-seconds", "0",
		"--disable-inspector",
	)
	server.cmd.Stdout = &server.output
	server.cmd.Stderr = &server.output
	if err := server.cmd.Start(); err != nil {
		server.Close()
		return nil, fmt.Errorf("start Lading: %w", err)
	}
	if err := server.waitReady(); err != nil {
		server.Close()
		return nil, err
	}
	return server, nil
}

func (s *ladingServer) waitReady() error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(s.endpoint)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if s.cmd.ProcessState != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("Lading endpoint %s did not become ready: %s", s.endpoint, s.output.String())
}

func (s *ladingServer) body() ([]byte, error) {
	response, err := http.Get(s.endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Lading endpoint returned %s", response.Status)
	}
	return io.ReadAll(response.Body)
}

func (s *ladingServer) Close() {
	if s == nil {
		return
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}
}
