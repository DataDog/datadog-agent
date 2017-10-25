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

func TestRunAll(t *testing.T) {

	diagnosis.Register("failing", func() error { return errors.New("fail") })
	diagnosis.Register("succeeding", func() error { return nil })

	w := &bytes.Buffer{}
	RunAll(w)

	expected := `=== Running failing diagnosis ===
===> FAIL

=== Running succeeding diagnosis ===
===> PASS

`
	if result := w.String(); result != expected {
		t.Errorf("Got: %v Expected: %v", result, expected)
	}
}
