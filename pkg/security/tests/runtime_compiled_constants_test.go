// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestFallbackConstants(t *testing.T) {
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	config := test.config
	config.RuntimeCompiledConstantsIsSet = true
	config.EnableRuntimeCompiledConstants = false

	withoutRC, err := probe.GetOffsetConstants(config, test.probe)
	if err != nil {
		t.Error(err)
	}

	constantfetch.ClearConstantsCache()

	config.EnableRuntimeCompiledConstants = true
	withRC, err := probe.GetOffsetConstants(config, test.probe)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, withoutRC, withRC)
}
