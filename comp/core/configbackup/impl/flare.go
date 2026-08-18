// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configbackupimpl

import (
	"context"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// FlareProvider adds the config backup manifests and occurrence log to a
// flare, so support can see what changed and when. The archives themselves
// are never added: they contain verbatim configuration copies including
// secrets.
type FlareProvider struct {
	Config config.Component
}

// ProvideFlare is the flare callback.
func (p *FlareProvider) ProvideFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	srcDir, err := resolveSrcDir(p.Config)
	if err != nil {
		// No config directory, nothing to add.
		return nil
	}
	backupDir := backupDirFromConfig(p.Config, srcDir)

	if data, err := os.ReadFile(filepath.Join(backupDir, startsLogName)); err == nil {
		if err := fb.AddFile("config-backups/starts.jsonl", data); err != nil {
			return err
		}
	}

	manifests, err := ReadManifests(backupDir)
	if err != nil {
		return nil
	}
	for _, m := range manifests {
		data, err := os.ReadFile(filepath.Join(backupDir, m.Digest+manifestSuffix))
		if err != nil {
			continue
		}
		if err := fb.AddFile("config-backups/"+m.Digest+manifestSuffix, data); err != nil {
			return err
		}
	}
	return nil
}
