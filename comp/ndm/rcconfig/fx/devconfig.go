// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fx

import (
	"fmt"
	"os"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const devConfigKey = "network_devices.remote_config.dev_config_file"

// devConfigPath is the Remote Configuration path a development document is applied under.
const devConfigPath = "datadog/0/NDM_CONFIG/dev-config/config"

// updater applies Remote Configuration documents, as ndm.Provider does.
type updater interface {
	Update(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

// applyDevConfig feeds the document in path to up as if Remote Configuration had delivered it.
func applyDevConfig(path string, up updater, logComp log.Component) error {
	document, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", devConfigKey, err)
	}

	logComp.Warnf("ndm: applying the development config %s, this is not Remote Configuration data", path)
	up.Update(
		map[string]state.RawConfig{devConfigPath: {Config: document}},
		func(_ string, status state.ApplyStatus) {
			if status.State == state.ApplyStateError {
				logComp.Errorf("ndm: the development config %s was rejected: %s", path, status.Error)
				return
			}
			logComp.Infof("ndm: the development config %s was applied", path)
		},
	)
	return nil
}
