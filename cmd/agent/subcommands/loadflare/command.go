// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package loadflare implements 'agent load-flare'.
package loadflare

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const route = "/agent/logs/load-characterization"

type cliParams struct {
	*command.GlobalParams
	context  context.Context
	duration time.Duration
	output   string
}

type artifact struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	SHA256        string `json:"sha256"`
}

type manifest struct {
	SchemaVersion     int            `json:"schema_version"`
	Kind              string         `json:"kind"`
	CreatedAt         time.Time      `json:"created_at"`
	Agent             map[string]any `json:"agent"`
	ObservationWindow map[string]any `json:"observation_window"`
	Artifacts         []artifact     `json:"artifacts"`
}

type archiveConfig interface {
	AllSettingsWithoutDefaultOrSecrets() map[string]interface{}
}

// Commands returns the load-flare command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{GlobalParams: globalParams, duration: 15 * time.Minute}
	cmd := &cobra.Command{
		Use:   "load-flare",
		Short: "Characterize logs load and create a local replay bundle",
		Args:  cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if params.duration < time.Second || params.duration > 24*time.Hour {
				return characterization.ErrInvalidDuration
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			params.context = cmd.Context()
			return fxutil.OneShot(run,
				fx.Supply(params),
				fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	cmd.Flags().DurationVarP(&params.duration, "duration", "d", params.duration, "Bounded characterization duration (1s through 24h)")
	cmd.Flags().StringVarP(&params.output, "output", "o", "", "Output archive path")
	return []*cobra.Command{cmd}
}

func run(params *cliParams, config config.Component, client ipc.HTTPClient) error {
	endpointURL, err := agentURL(config)
	if err != nil {
		return err
	}
	requestBody, _ := json.Marshal(map[string]float64{"duration_seconds": params.duration.Seconds()})
	response, err := client.Post(endpointURL, "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("could not start logs characterization: %w", err)
	}
	var started characterization.Snapshot
	if err := json.Unmarshal(response, &started); err != nil {
		return fmt.Errorf("could not decode start response: %w", err)
	}
	fmt.Printf("Characterizing logs load for %s (session %s)...\n", params.duration, started.SessionID)

	timer := time.NewTimer(params.duration)
	defer timer.Stop()
	var interrupted error
	select {
	case <-timer.C:
	case <-params.context.Done():
		interrupted = params.context.Err()
	}

	stopURL := endpointURL + "?session_id=" + started.SessionID
	request, err := http.NewRequest(http.MethodDelete, stopURL, nil)
	if err != nil {
		return err
	}
	response, err = client.Do(request)
	if err != nil {
		return fmt.Errorf("could not stop logs characterization: %w", err)
	}
	var completed characterization.Snapshot
	if err := json.Unmarshal(response, &completed); err != nil {
		return fmt.Errorf("could not decode characterization result: %w", err)
	}

	output := params.output
	if output == "" {
		output = fmt.Sprintf("datadog-agent-load-flare-%s.zip", time.Now().UTC().Format("20060102T150405Z"))
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := writeArchive(output, completed, config); err != nil {
		return err
	}
	fmt.Printf("Load flare written to %s\n", output)
	if interrupted != nil {
		return fmt.Errorf("characterization stopped early: %w", interrupted)
	}
	return nil
}

func agentURL(config config.Component) (string, error) {
	address, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}
	return "https://" + net.JoinHostPort(address, strconv.Itoa(config.GetInt("cmd_port"))) + route, nil
}

func writeArchive(output string, snapshot characterization.Snapshot, config archiveConfig) error {
	observation := map[string]any{
		"schema_version": 1,
		"kind":           "logs-characterization-observation",
		"source":         map[string]any{"id": snapshot.SessionID, "label": "running-agent", "format": "agent-session-v1"},
		"measurement": map[string]any{
			"start_unix_ns":    snapshot.StartedAt.UnixNano(),
			"end_unix_ns":      snapshot.EndedAt.UnixNano(),
			"duration_seconds": snapshot.EndedAt.Sub(snapshot.StartedAt).Seconds(),
		},
		"toolchain":     map[string]any{"agent_version": version.AgentVersion, "agent_commit": version.Commit},
		"agent_summary": snapshot,
		"rate_windows":  snapshot.RateWindows,
		"lifecycle":     snapshot.Lifecycle,
	}
	lading, report := inferLading(snapshot)
	resources := collectResources()
	settings, err := json.MarshalIndent(config.AllSettingsWithoutDefaultOrSecrets(), "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode Agent configuration: %w", err)
	}
	settings, err = scrubber.ScrubBytes(settings)
	if err != nil {
		return fmt.Errorf("could not scrub Agent configuration: %w", err)
	}

	files := map[string][]byte{}
	for name, value := range map[string]any{
		"observation.json":      observation,
		"inference-report.json": report,
		"resources.json":        resources,
	} {
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		files[name] = append(encoded, '\n')
	}
	files["agent-config.json"] = append(settings, '\n')
	if len(lading) > 0 {
		files["lading.yaml"] = lading
	}
	if bytes.Contains(lading, []byte(datadogJSONTemplatePath)) {
		files["load-flare-assets/datadog-json-template.yaml"] = []byte(datadogJSONTemplate)
	}
	files["README.txt"] = []byte("This local load flare contains bounded aggregate logs observations. It contains no raw log messages or source paths.\n")

	createdAt := time.Now().UTC()
	artifacts := make([]artifact, 0, len(files))
	for name, content := range files {
		digest := sha256.Sum256(content)
		item := artifact{Path: name, Kind: strings.TrimSuffix(name, filepath.Ext(name)), SHA256: hex.EncodeToString(digest[:])}
		if strings.HasSuffix(name, ".json") && name != "agent-config.json" && name != "resources.json" {
			item.SchemaVersion = 1
		}
		artifacts = append(artifacts, item)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	bundleManifest := manifest{
		SchemaVersion:     1,
		Kind:              "datadog-agent-load-flare",
		CreatedAt:         createdAt,
		Agent:             map[string]any{"version": version.AgentVersion, "commit": version.Commit},
		ObservationWindow: map[string]any{"started_at": snapshot.StartedAt, "ended_at": snapshot.EndedAt, "requested_duration_seconds": snapshot.RequestedDurationSeconds},
		Artifacts:         artifacts,
	}
	manifestBytes, err := json.MarshalIndent(bundleManifest, "", "  ")
	if err != nil {
		return err
	}
	files["manifest.json"] = append(manifestBytes, '\n')
	return writeZipAtomic(output, files, createdAt)
}

func writeZipAtomic(output string, files map[string][]byte, modified time.Time) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".load-flare-*.zip")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	archive := zip.NewWriter(temporary)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(modified)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := writer.Write(files[name]); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0600); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return err
	}
	return nil
}

func collectResources() map[string]any {
	resources := map[string]any{
		"schema_version": 1,
		"kind":           "datadog-agent-load-flare-resources",
		"goos":           runtime.GOOS,
		"goarch":         runtime.GOARCH,
		"logical_cpus":   runtime.NumCPU(),
		"gomaxprocs":     runtime.GOMAXPROCS(0),
	}
	cgroup := map[string]string{}
	for _, path := range []string{"/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.high", "/sys/fs/cgroup/cpuset.cpus.effective"} {
		if value, err := os.ReadFile(path); err == nil {
			cgroup[filepath.Base(path)] = strings.TrimSpace(string(value))
		}
	}
	if len(cgroup) > 0 {
		resources["cgroup_v2"] = cgroup
	}
	return resources
}
