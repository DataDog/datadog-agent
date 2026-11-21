// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

func TestMacOSExportsQuotesValues(t *testing.T) {
	params := &installparams.Params{
		APIKey: "k",
		ExtraEnvVars: map[string]string{
			"DD_SPACE": "a b",
			"DD_QUOTE": "it's",
			"DD_NL":    "a\nb",
			"DD_SUB":   "$(touch injected)",
		},
	}

	got, err := macOSExports(params)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"DD_API_KEY='k'",
		"DD_NL='a\nb'",
		"DD_QUOTE='it'\\''s'",
		"DD_SPACE='a b'",
		"DD_SUB='$(touch injected)'",
	}, got)
}

func TestMacOSExportsRejectsInvalidKeys(t *testing.T) {
	for _, key := range []string{"A-B", "1A", "A=VALUE"} {
		t.Run(key, func(t *testing.T) {
			_, err := macOSExports(&installparams.Params{ExtraEnvVars: map[string]string{key: "true"}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
		})
	}
}
