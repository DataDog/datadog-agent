// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type Task struct {
	Data struct {
		ID         string      `json:"id,omitempty"`
		Type       string      `json:"type,omitempty"`
		Attributes *Attributes `json:"attributes,omitempty"`
	} `json:"data,omitempty"`
	Raw []byte `json:"-"`
}

type Attributes struct {
	Name                  string                                        `json:"name"`
	BundleID              string                                        `json:"bundle_id"`
	SecDatadogHeaderValue string                                        `json:"sec_datadog_header_value"`
	Inputs                map[string]interface{}                        `json:"inputs"`
	OrgId                 int64                                         `json:"org_id"`
	JobId                 string                                        `json:"job_id"`
	SignedEnvelope        *privateactions.RemoteConfigSignatureEnvelope `json:"signed_envelope"`
	ConnectionInfo        *privateactions.ConnectionInfo                `json:"connection_info"`
}

func (task *Task) GetFQN() string {
	return utils.QualifyName(task.Data.Attributes.BundleID, task.Data.Attributes.Name)
}

func (task *Task) Validate() error {
	if task == nil || task.Data.Attributes == nil {
		return fmt.Errorf("empty task provided")
	}
	if task.Data.Attributes.JobId == "" {
		return fmt.Errorf("no JobId provided")
	}
	return nil
}

func ExtractInputs[T any](task *Task) (T, error) {
	var res T
	jsonInputs, err := json.Marshal(task.Data.Attributes.Inputs)
	if err != nil {
		return res, fmt.Errorf("error marshaling inputs to JSON: %w, inputs: %v", err, task.Data.Attributes.Inputs)
	}
	if err := json.Unmarshal(jsonInputs, &res); err != nil {
		return res, fmt.Errorf("error unmarshaling inputs from JSON: %w, inputs: %s", err, string(jsonInputs))
	}
	return res, nil
}
