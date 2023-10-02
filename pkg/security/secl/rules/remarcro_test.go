// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReMacro(t *testing.T) {
	t.Run("test1", func(t *testing.T) {
		def := &PolicyDef{
			Rules: []*RuleDefinition{
				{
					ID:         "111",
					Expression: `exec.file.path in ["aaa", "bbb", "ccc"]`,
				},
				{
					ID:         "222",
					Expression: `exec.args in ["123", "567"] && exec.file.path in ["aaa", "bbb", "ccc"]`,
				},
			},
		}

		ReMacro(def)

		assert.Equal(t, 1, len(def.Macros))
		assert.Equal(t, `["aaa", "bbb", "ccc"]`, def.Macros[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.file.path in %s`, def.Macros[0].ID), def.Rules[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.args in ["123", "567"] && exec.file.path in %s`, def.Macros[0].ID), def.Rules[1].Expression)
	})

	t.Run("test2", func(t *testing.T) {
		def := &PolicyDef{
			Rules: []*RuleDefinition{
				{
					ID:         "111",
					Expression: `exec.file.path in ["aaa", "bbb", "ccc"]`,
				},
				{
					ID:         "222",
					Expression: `exec.args in ["123", "567"] && exec.file.path in ["aaa", "bbb", "ccc"]`,
				},
				{
					ID:         "222",
					Expression: `exec.file.path in ["aaa", "bbb", "ccc"] || exec.args in ["123", "567"]`,
				},
			},
		}

		ReMacro(def)

		assert.Equal(t, 2, len(def.Macros))
		assert.Equal(t, `["aaa", "bbb", "ccc"]`, def.Macros[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.file.path in %s`, def.Macros[0].ID), def.Rules[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.args in %s && exec.file.path in %s`, def.Macros[1].ID, def.Macros[0].ID), def.Rules[1].Expression)
	})

	t.Run("test3", func(t *testing.T) {
		def := &PolicyDef{
			Rules: []*RuleDefinition{
				{
					ID:         "111",
					Expression: `exec.file.path in ["aaa", r"[a-z]*", "ccc"]`,
				},
				{
					ID:         "222",
					Expression: `exec.args in ["123", "567"] && exec.file.path in ["aaa", r"[a-z]*", "ccc"]`,
				},
			},
		}

		ReMacro(def)

		assert.Equal(t, 1, len(def.Macros))
		assert.Equal(t, `["aaa", r"[a-z]*", "ccc"]`, def.Macros[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.file.path in %s`, def.Macros[0].ID), def.Rules[0].Expression)
		assert.Equal(t, fmt.Sprintf(`exec.args in ["123", "567"] && exec.file.path in %s`, def.Macros[0].ID), def.Rules[1].Expression)
	})

	t.Run("test4", func(t *testing.T) {
		def := &PolicyDef{
			Rules: []*RuleDefinition{
				{
					ID:         "111",
					Expression: `exec.file.path == "/var/log/[]123`,
				},
				{
					ID:         "222",
					Expression: `exec.args in ["123", "567"] && exec.file.path == "/var/log/[]123`,
				},
			},
		}

		ReMacro(def)

		assert.Equal(t, 0, len(def.Macros))
		assert.Equal(t, `exec.file.path == "/var/log/[]123`, def.Rules[0].Expression)
		assert.Equal(t, `exec.args in ["123", "567"] && exec.file.path == "/var/log/[]123`, def.Rules[1].Expression)
	})
}
