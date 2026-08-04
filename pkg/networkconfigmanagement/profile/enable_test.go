// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package profile

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnableCommand_EffectivePasswordPrompt(t *testing.T) {
	t.Run("nil receiver returns default", func(t *testing.T) {
		var ec *EnableCommand
		assert.Equal(t, DefaultEnablePasswordPrompt, ec.EffectivePasswordPrompt())
	})
	t.Run("unset PasswordPrompt returns default", func(t *testing.T) {
		ec := &EnableCommand{Command: "enable"}
		assert.Equal(t, DefaultEnablePasswordPrompt, ec.EffectivePasswordPrompt())
	})
	t.Run("custom PasswordPrompt is honored", func(t *testing.T) {
		custom := regexp.MustCompile("custom prompt")
		ec := &EnableCommand{Command: "enable", PasswordPrompt: custom}
		assert.Equal(t, custom, ec.EffectivePasswordPrompt())
	})
}
