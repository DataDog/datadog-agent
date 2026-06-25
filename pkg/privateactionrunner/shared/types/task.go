// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	actionsclientpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/actionsclient"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
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
	Name                  string                                          `json:"name"`
	BundleID              string                                          `json:"bundle_id"`
	SecDatadogHeaderValue string                                          `json:"sec_datadog_header_value"`
	Inputs                map[string]interface{}                          `json:"inputs"`
	Client                actionsclientpb.Client                          `json:"client"`
	OrgId                 int64                                           `json:"org_id"`
	JobId                 string                                          `json:"job_id"`
	SignedEnvelope        *privateactionspb.RemoteConfigSignatureEnvelope `json:"signed_envelope"`
	ConnectionInfo        *privateactionspb.ConnectionInfo                `json:"connection_info"`
	TraceId               uint64                                          `json:"trace_id,omitempty"`
	SpanId                uint64                                          `json:"span_id,omitempty"`
}

// TimeoutSeconds returns the timeout from the task inputs if present, positive, and within int32
// range. Returns nil (fall back to config) for missing, non-integer, non-positive, or out-of-range values.
func (task *Task) TimeoutSeconds() *int32 {
	if task.Data.Attributes == nil {
		return nil
	}
	v, ok := task.Data.Attributes.Inputs["timeout"]
	if !ok {
		return nil
	}

	var n int64
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			// Non-integer float (e.g. 1.5) — reject.
			return nil
		}
		n = int64(t)
	case int32:
		n = int64(t)
	case int64:
		n = t
	case int:
		n = int64(t)
	default:
		return nil
	}

	if n <= 0 || n > math.MaxInt32 {
		return nil
	}
	val := int32(n)
	return &val
}

func (task *Task) GetFQN() string {
	return fmt.Sprintf("%s.%s", task.Data.Attributes.BundleID, task.Data.Attributes.Name)
}

func (task *Task) Validate() error {
	if task == nil || task.Data.Attributes == nil {
		return errors.New("empty task provided")
	}
	if task.Data.Attributes.JobId == "" {
		return errors.New("no JobId provided")
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
