// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// RCAddConfig pushes a Remote Config entry to fakeintake. data must be a valid
// JSON document; fakeintake re-marshals it for stable storage.
func (c *Client) RCAddConfig(orgID, product, configID, configName string, data []byte) error {
	if !json.Valid(data) {
		return errors.New("data is not valid JSON")
	}
	body, err := json.Marshal(api.RCAddConfigRequest{
		OrgID:      orgID,
		Product:    product,
		ConfigID:   configID,
		ConfigName: configName,
		Data:       json.RawMessage(data),
	})
	if err != nil {
		return err
	}
	resp, err := http.Post(c.fakeIntakeURL+"/fakeintake/rc/config", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rc add: status %d: %s", resp.StatusCode, errBody)
	}
	return nil
}

// RCListConfigs returns every Remote Config entry stored on fakeintake.
func (c *Client) RCListConfigs() ([]api.RCConfig, error) {
	resp, err := http.Get(c.fakeIntakeURL + "/fakeintake/rc/configs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rc list: status %d: %s", resp.StatusCode, errBody)
	}
	var out []api.RCConfig
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// RCDeleteConfig removes the entry whose path key is "<org>/<product>/<config_id>/<config_name>".
func (c *Client) RCDeleteConfig(key string) error {
	req, err := http.NewRequest(http.MethodDelete, c.fakeIntakeURL+"/fakeintake/rc/config/"+key, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rc delete: status %d: %s", resp.StatusCode, errBody)
	}
	return nil
}

// RCStats returns counters and the current TUF root for fakeintake's RC repo.
// Use Polls / LastPoll to assert that the agent has actually reached out.
func (c *Client) RCStats() (api.RCStats, error) {
	var stats api.RCStats
	resp, err := http.Get(c.fakeIntakeURL + "/fakeintake/rc/stats")
	if err != nil {
		return stats, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return stats, fmt.Errorf("rc stats: status %d: %s", resp.StatusCode, errBody)
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return stats, err
	}
	return stats, nil
}
