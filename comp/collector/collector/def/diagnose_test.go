// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

type stubCheckWithDiagnoses struct {
	stub.StubCheck
	name      string
	diagnoses []diagnose.Diagnosis
	err       error
}

func (c *stubCheckWithDiagnoses) String() string { return c.name }
func (c *stubCheckWithDiagnoses) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return c.diagnoses, c.err
}

func TestGetInstanceDiagnosesStampsCheckName(t *testing.T) {
	t.Run("stamps CheckName when not set", func(t *testing.T) {
		ch := &stubCheckWithDiagnoses{
			name: "postgres",
			diagnoses: []diagnose.Diagnosis{
				{Status: diagnose.DiagnosisSuccess, Name: "conn", Diagnosis: "ok", Category: "dbm"},
			},
		}
		got := GetInstanceDiagnoses(ch)
		assert.Len(t, got, 1)
		assert.Equal(t, "postgres", got[0].CheckName)
		assert.Equal(t, "dbm", got[0].Category) // category unchanged
	})

	t.Run("does not overwrite existing CheckName", func(t *testing.T) {
		ch := &stubCheckWithDiagnoses{
			name: "postgres",
			diagnoses: []diagnose.Diagnosis{
				{Status: diagnose.DiagnosisSuccess, Name: "conn", Diagnosis: "ok", CheckName: "custom-name"},
			},
		}
		got := GetInstanceDiagnoses(ch)
		assert.Equal(t, "custom-name", got[0].CheckName)
	})

	t.Run("falls back category to check name when category empty", func(t *testing.T) {
		ch := &stubCheckWithDiagnoses{
			name: "mysql",
			diagnoses: []diagnose.Diagnosis{
				{Status: diagnose.DiagnosisSuccess, Name: "conn", Diagnosis: "ok"},
			},
		}
		got := GetInstanceDiagnoses(ch)
		assert.Equal(t, "mysql", got[0].Category)
		assert.Equal(t, "mysql", got[0].CheckName)
	})
}
