// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package format

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func TestOutputDiagnosisNameLine(t *testing.T) {
	cfg := diagnose.Config{Verbose: false}

	cases := []struct {
		name      string
		checkName string
		category  string
		want      string
	}{
		{"both set", "postgres", "dbm", "  PASS [postgres] [dbm] my-diag\n"},
		{"checkname only", "postgres", "", "  PASS [postgres] my-diag\n"},
		{"category only", "", "dbm", "  PASS [dbm] my-diag\n"},
		{"neither set", "", "", "  PASS my-diag\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			d := diagnose.Diagnosis{
				Status:    diagnose.DiagnosisSuccess,
				Name:      "my-diag",
				Diagnosis: "all good",
				CheckName: tc.checkName,
				Category:  tc.category,
			}
			outputDiagnosis(&buf, cfg, "PASS", 1, d)
			line := strings.Split(buf.String(), "\n")[1] + "\n"
			assert.Equal(t, tc.want, line)
		})
	}
}
