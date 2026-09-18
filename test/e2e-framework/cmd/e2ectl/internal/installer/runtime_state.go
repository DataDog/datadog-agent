// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

const runtimeStateKey = "_agent_runtime_volume"

func runtimeVolumeRecord(entry envstore.Entry) (localinfra.RuntimeVolume, bool, error) {
	expected := localinfra.AgentRuntimeVolume(entry.Dir, entry.Meta.CreatedAt)
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return expected, false, err
	}
	raw := meta[runtimeStateKey]
	if raw == nil || string(raw) == "null" {
		return expected, false, nil
	}
	var recorded localinfra.RuntimeVolume
	if err = json.Unmarshal(raw, &recorded); err != nil {
		return expected, false, err
	}
	if recorded != expected {
		return expected, false, fmt.Errorf("recorded Agent runtime volume belongs to another environment generation")
	}
	return expected, true, nil
}

// Intermediate receiver implementations used a host bind mount. Never silently
// discard or copy live RC state into a fresh volume. Empty scratch directories
// and original installs without this directory remain usable.
func rejectLegacyRuntimeState(entry envstore.Entry) error {
	files, err := os.ReadDir(filepath.Join(entry.Dir, "agent-run"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || len(files) > 0 {
		return fmt.Errorf("legacy bind-mounted agent-run state requires explicit migration/recreation before install/update; receiver-only apply can reuse it; root-owned state may require authorized narrowly scoped cleanup (stop --force does not fix permissions)")
	}
	return nil
}
func (b *Binary) preflightRuntimeState(entry envstore.Entry) error {
	if err := rejectLegacyRuntimeState(entry); err != nil {
		return err
	}
	v, recorded, err := runtimeVolumeRecord(entry)
	if err != nil {
		return err
	}
	if recorded {
		return localinfra.VerifyRuntimeVolume(v, b.dockerCommand)
	}
	return nil
}
func (b *Binary) verifyRuntimeState(entry envstore.Entry) error {
	v, recorded, err := runtimeVolumeRecord(entry)
	if err != nil {
		return err
	}
	if !recorded {
		return fmt.Errorf("installed named runtime volume receipt missing; refusing empty-state reuse")
	}
	return localinfra.VerifyRuntimeVolume(v, b.dockerCommand)
}
func (b *Binary) prepareRuntimeState(entry envstore.Entry) error {
	v, recorded, err := runtimeVolumeRecord(entry)
	if err != nil {
		return err
	}
	if recorded {
		return localinfra.VerifyRuntimeVolume(v, b.dockerCommand)
	}
	if err = localinfra.EnsureRuntimeVolume(v, b.dockerCommand); err != nil {
		return err
	}
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{runtimeStateKey: v})
}
