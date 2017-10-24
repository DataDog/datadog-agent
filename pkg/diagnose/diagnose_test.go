// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package diagnose

import (
	"bytes"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

type dummySucceedingDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dummySucceedingDiagnosis) Diagnose() error {
	return nil
}

type dummyFailingDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dummyFailingDiagnosis) Diagnose() error {
	return errors.New("fail")
}

func TestDiagnose(t *testing.T) {

	diagnosis.Register("failing", new(dummyFailingDiagnosis))
	diagnosis.Register("succeeding", new(dummySucceedingDiagnosis))

	w := &bytes.Buffer{}
	Diagnose(w)

	expected := `=== Running failing diagnosis ===
===> FAIL

=== Running succeeding diagnosis ===
===> PASS

`
	if result := w.String(); result != expected {
		t.Errorf("Got: %v Expected: %v", result, expected)
	}
}
