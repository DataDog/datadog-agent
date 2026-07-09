// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package exampletaskmanagerimpl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	triggerTaskName  = "trigger"
	exampleCheckName = "example"
	oneShotInstance  = "min_collection_interval: 0\n"
)

// Example payload for the example task manager
//
//	{
//	  "tasks": [
//	    {
//	      "name": "trigger"
//	    }
//	  ]
//	}
type rcPayload struct {
	Tasks []rcTask `json:"tasks"`
}

type rcTask struct {
	Name string `json:"name"`
}

func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	if len(updates) == 0 {
		c.log.Debugf("exampletaskmanager: RC DEBUG update with 0 active config(s)")
		return
	}

	changes := integration.ConfigChanges{}
	for path, rawConfig := range updates {
		var payload rcPayload
		if err := json.Unmarshal(rawConfig.Config, &payload); err != nil {
			c.log.Warnf("exampletaskmanager: failed to unmarshal DEBUG config %s: %v", path, err)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}

		if !hasTriggerTask(payload) {
			c.log.Debugf("exampletaskmanager: skipping DEBUG config %s: no %q task", configShortName(path), triggerTaskName)
			applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		// Schedule the check configuration for the example check
		changes.Schedule = append(changes.Schedule, buildCheckConfig(c.String(), path))
		c.log.Infof("exampletaskmanager: scheduled one-shot %q check for %s", exampleCheckName, configShortName(path))
		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	if !changes.IsEmpty() {
		c.sendChanges(changes)
	}
}

func hasTriggerTask(payload rcPayload) bool {
	for _, task := range payload.Tasks {
		if task.Name == triggerTaskName {
			return true
		}
	}
	return false
}

func buildCheckConfig(providerName, path string) integration.Config {
	return integration.Config{
		Name:       exampleCheckName,
		Source:     fmt.Sprintf("%s:%s", providerName, configShortName(path)),
		Instances:  []integration.Data{integration.Data(oneShotInstance)},
		InitConfig: integration.Data("{}"),
	}
}

func configShortName(path string) string {
	const prefix = "datadog/2/DEBUG/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		rest := path[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	return path
}
