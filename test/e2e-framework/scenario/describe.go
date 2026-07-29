// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

// ProtocolVersion is the version of the describe/create/action/destroy contract
// between the (version-stable) service and per-commit binaries. Bump on breaking
// changes only.
const ProtocolVersion = 1

// ScenarioDescription is the JSON-serializable description of one scenario.
type ScenarioDescription struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      Schema            `json:"params"`
	Actions     map[string]Schema `json:"actions"`
}

// Description is the full machine-readable registry description.
type Description struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	Scenarios       []ScenarioDescription `json:"scenarios"`
}

// Describe builds the registry description for `scenariorun describe --json`.
func Describe() (Description, error) {
	d := Description{ProtocolVersion: ProtocolVersion}
	for _, r := range List() {
		ps, err := r.ParamsSchema()
		if err != nil {
			return Description{}, err
		}
		as, err := r.ActionSchemas()
		if err != nil {
			return Description{}, err
		}
		d.Scenarios = append(d.Scenarios, ScenarioDescription{
			Name:        r.Name(),
			Description: r.Description(),
			Params:      ps,
			Actions:     as,
		})
	}
	return d, nil
}
