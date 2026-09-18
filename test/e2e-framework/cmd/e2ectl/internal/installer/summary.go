// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"os"
)

// ArtifactDisplay keeps legacy artifact-format presentation with installers,
// not in command dispatch. New records also carry the independent receipt ID.
func ArtifactDisplay(meta envstore.Meta) string {
	if !meta.AgentInstalled {
		return "-"
	}
	if meta.AgentImage != "" {
		return meta.AgentImage
	}
	if meta.AgentVersion != "" {
		return meta.AgentVersion
	}
	return "binary"
}

// ListArtifactDisplay shows recorded mutation outcomes independently of the
// success index: failed first installs have no AgentInstalled flag or receipt ID.
func ListArtifactDisplay(entry envstore.Entry) string {
	phase, err := RecordedArtifactPhase(entry)
	if err != nil && (!os.IsNotExist(err) || entry.Meta.AgentArtifactID != "") {
		return "artifact state unreadable"
	}
	if phase != "" && phase != "installed" {
		return "artifact " + phase + " (see snapshot)"
	}
	if phase == "installed" && !entry.Meta.AgentInstalled {
		return "artifact installed (recorded; readiness unknown)"
	}
	display := ArtifactDisplay(entry.Meta)
	if entry.Meta.AgentInstalled {
		if id := entry.Meta.AgentArtifactID; id != "" {
			display += " [receipt:" + id[:min(12, len(id))] + "]"
		} else {
			display += " [legacy/unverified]"
		}
	}
	return display
}

// RecordedArtifactPhase is a local-files observation, not a health probe.
func RecordedArtifactPhase(entry envstore.Entry) (string, error) {
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return "", err
	}
	raw := meta["_agent_artifact"]
	if raw == nil || string(raw) == "null" {
		return "", nil
	}
	var state artifactState
	if err = json.Unmarshal(raw, &state); err != nil {
		return "", err
	}
	return state.Phase, nil
}

// InstalledSummary returns recorded output identity, falling back only for
// legacy installs without any receipt. It performs no build/Docker inspection.
func InstalledSummary(inst Installer, cfg *config.File, entry envstore.Entry) (agentbuild.Summary, error) {
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return agentbuild.Summary{}, err
	}
	if raw := meta["_agent_artifact"]; raw != nil && string(raw) != "null" {
		var state artifactState
		if err = json.Unmarshal(raw, &state); err != nil {
			return agentbuild.Summary{}, err
		}
		if state.Phase != "installed" {
			return agentbuild.Summary{}, fmt.Errorf("artifact activation did not finish")
		}
		return state.Result.Summary(), nil
	}
	version, image, err := inst.Artifact(cfg)
	return agentbuild.Summary{Version: version, Image: image}, err
}
