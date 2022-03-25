// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package serializerexporter

import (
	"fmt"

	"go.opentelemetry.io/collector/config"
	"go.uber.org/multierr"
)

var _ error = (*renameError)(nil)

// renameError is an error related to a renamed setting.
type renameError struct {
	// oldName of the configuration option.
	oldName string
	// newName of the configuration option.
	newName string
	// oldRemovedIn is the version where the old config option will be removed.
	oldRemovedIn string
	// updateFn updates the configuration to map the old value into the new one.
	// It must only be called when the old value is set and is not the default.
	updateFn func(*exporterConfig)
}

// List of settings that are deprecated.
var renamedSettings = []renameError{
	{
		oldName:      "metrics::send_monotonic_counter",
		newName:      "metrics::sums::cumulative_monotonic_mode",
		oldRemovedIn: "v7.37",
		updateFn: func(c *exporterConfig) {
			if c.Metrics.SendMonotonic {
				c.Metrics.SumConfig.CumulativeMonotonicMode = CumulativeMonotonicSumModeToDelta
			} else {
				c.Metrics.SumConfig.CumulativeMonotonicMode = CumulativeMonotonicSumModeRawValue
			}
		},
	},
}

// Error implements the error interface.
func (e renameError) Error() string {
	return fmt.Sprintf(
		"%q has been deprecated in favor of %q and will be removed in %s",
		e.oldName,
		e.newName,
		e.oldRemovedIn,
	)
}

// Check if the deprecated option is being used.
// Error out if both the old and new options are being used.
func (e renameError) Check(configMap *config.Map) (bool, error) {
	if configMap.IsSet(e.oldName) && configMap.IsSet(e.newName) {
		return false, fmt.Errorf("%q and %q can't be both set at the same time: use %q only instead", e.oldName, e.newName, e.newName)
	}
	return configMap.IsSet(e.oldName), nil
}

// UpdateCfg to move the old configuration value into the new one.
func (e renameError) UpdateCfg(cfg *exporterConfig) {
	e.updateFn(cfg)
}

// handleRenamedSettings for a given configuration map.
// Error out if any pair of old-new options are set at the same time.
func handleRenamedSettings(configMap *config.Map, cfg *exporterConfig) (warnings []error, err error) {
	for _, renaming := range renamedSettings {
		isOldNameUsed, errCheck := renaming.Check(configMap)
		err = multierr.Append(err, errCheck)

		if errCheck == nil && isOldNameUsed {
			warnings = append(warnings, renaming)
			// only update config if old name is in use
			renaming.UpdateCfg(cfg)
		}
	}
	return
}
