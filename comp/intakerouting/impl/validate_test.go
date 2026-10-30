// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package intakeroutingimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

func TestValidateOverride(t *testing.T) {
	valid := []string{
		"acme.agent-http-intake.logs.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.com.",
		"ACME.Agent-HTTP-Intake.logs.datadoghq.com",
		"agent-http-intake.logs.us3.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.eu",
		"acme.agent-http-intake.logs.ddog-gov.com",
		"acme.agent-http-intake.logs.datad0g.com",
		// Any official site is accepted, not only the site of the default endpoint.
		"acme.agent-http-intake.logs.ap1.datadoghq.com",
	}
	for _, host := range valid {
		assert.NoError(t, validateOverride(host, 0), host)
	}
	assert.NoError(t, validateOverride(valid[0], 443))
	assert.NoError(t, validateOverride(valid[0], 8443))

	invalid := []string{
		"",
		"10.0.0.1",
		"::1",
		"[::1]",
		"datadoghq.com",
		"us3.datadoghq.com.",
		"evil.example.com",
		"notdatadoghq.com",
		"datadoghq.com.evil.example",
		"https://acme.agent-http-intake.logs.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.com/path",
		"user@acme.agent-http-intake.logs.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.com:443",
		"acme..datadoghq.com",
		"-acme.datadoghq.com",
		"acme_corp.datadoghq.com",
		"acme corp.datadoghq.com",
	}
	for _, host := range invalid {
		assert.Error(t, validateOverride(host, 0), host)
	}
	assert.Error(t, validateOverride(valid[0], -1))
	assert.Error(t, validateOverride(valid[0], 65536))
}

func TestSameEndpoint(t *testing.T) {
	a := intakerouting.Endpoint{Host: "agent-http-intake.logs.datadoghq.com"}
	assert.True(t, sameEndpoint(a, intakerouting.Endpoint{Host: "agent-http-intake.logs.datadoghq.com.", Port: 443}))
	assert.True(t, sameEndpoint(a, intakerouting.Endpoint{Host: "AGENT-HTTP-INTAKE.LOGS.DATADOGHQ.COM"}))
	assert.False(t, sameEndpoint(a, intakerouting.Endpoint{Host: "agent-http-intake.logs.datadoghq.com", Port: 8443}))
	assert.False(t, sameEndpoint(a, intakerouting.Endpoint{Host: "acme.agent-http-intake.logs.datadoghq.com"}))
}
