// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package params

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
)

// FakeintakeParams is the reusable fakeintake component. Its tagged fields
// appear on the scenario CLI surface and convert to fakeintake.Option slices
// consumable by ec2.WithFakeIntakeOptions (and equivalent cross-cloud helpers).
//
// Note: AdvancedOptions uses the fakeintake.Option type from
// scenarios/aws/fakeintake (= func(*fakeintake.Params) error), which is the
// same type accepted by ec2.WithFakeIntakeOptions. Cross-cloud equivalents
// (azure, gcp) expose the same structural type; callers targeting those
// providers should adapt as needed.
type FakeintakeParams struct {
	Enabled bool `scenario:"name=use-fakeintake,default=false,help=Provision a fakeintake and point the agent at it"`

	// AdvancedOptions is a Go-only escape hatch for the full fakeintake surface.
	// These options are appended verbatim when ToOptions is called.
	AdvancedOptions []fakeintake.Option `scenario:"-"`
}

// ToOptions converts the component to a slice of fakeintake options.
// It always returns the AdvancedOptions as-is (no error path today).
func (f FakeintakeParams) ToOptions() ([]fakeintake.Option, error) {
	return f.AdvancedOptions, nil
}
