// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package healthcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestSynthesizeAllowlist(t *testing.T) {
	cfg := integration.RemediationConfig{
		Steps:           []integration.RemediationStep{{Command: "echo $(cat /tmp/input) | tee /tmp/output; echo done"}, {Command: "systemctl restart example.service"}},
		AllowedPaths:    []string{"/tmp/input:ro", "/tmp/output:rw"},
		AllowedServices: map[string][]string{"example.service": {"restart"}},
	}
	policy, err := synthesizeAllowlist(cfg)
	require.NoError(t, err)
	assert.Equal(t, cfg.AllowedPaths, policy.AllowedPaths)
	assert.Equal(t, []string{"rshell:cat", "rshell:echo", "rshell:systemctl", "rshell:tee"}, policy.AllowedCommands)
	assert.Equal(t, []string{"restart"}, policy.AllowedServices["example.service"].Actions)
	cfg.AllowedServices["example.service"][0] = "stop"
	cfg.AllowedPaths[0] = "changed"
	assert.Equal(t, "restart", policy.AllowedServices["example.service"].Actions[0])
	assert.Equal(t, "/tmp/input:ro", policy.AllowedPaths[0])
}

func TestSynthesizeAllowlistDefaultDeny(t *testing.T) {
	policy, err := synthesizeAllowlist(integration.RemediationConfig{Steps: []integration.RemediationStep{{Command: "systemctl restart example"}}})
	require.NoError(t, err)
	assert.Empty(t, policy.AllowedPaths)
	assert.Empty(t, policy.AllowedServices, "a service command must not implicitly authorize a service")
	assert.Equal(t, []string{"rshell:systemctl"}, policy.AllowedCommands)
}

func TestSynthesizeAllowlistRejectsAmbiguousPolicy(t *testing.T) {
	for _, command := range []string{"", "$(echo cat) /tmp/input", "$COMMAND /tmp/input", "c*t /tmp/input", "sudo cat /tmp/input", "echo $(", "rshell:*", "/usr/bin/echo hello", "sudo -n echo no"} {
		t.Run(command, func(t *testing.T) {
			_, err := synthesizeAllowlist(integration.RemediationConfig{Steps: []integration.RemediationStep{{Command: command}}})
			assert.Error(t, err)
		})
	}
	for _, cfg := range []integration.RemediationConfig{
		{},
		{AllowedPaths: []string{"/tmp/*"}},
		{AllowedServices: map[string][]string{"*": {"restart"}}},
		{AllowedServices: map[string][]string{"example": {"*"}}},
	} {
		_, err := synthesizeAllowlist(cfg)
		assert.Error(t, err)
	}
}

func TestRegistryClonesServicePolicy(t *testing.T) {
	cfg := healthConfig()
	cfg.Remediation.AllowedServices = map[string][]string{"example": {"restart"}}
	id := registerTestCheck(t, cfg)
	cfg.Remediation.AllowedServices["example"][0] = "stop"
	got, ok := Lookup(id)
	require.True(t, ok)
	assert.Equal(t, []string{"restart"}, got.Remediation.AllowedServices["example"])
	got.Remediation.AllowedServices["example"][0] = "start"
	got, _ = Lookup(id)
	assert.Equal(t, []string{"restart"}, got.Remediation.AllowedServices["example"])
}
