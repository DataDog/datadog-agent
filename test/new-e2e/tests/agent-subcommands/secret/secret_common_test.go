// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"

	"github.com/stretchr/testify/assert"
)

type baseSecretSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func (v *baseSecretSuite) TestAgentSecretNotEnabledByDefault() {
	secret := v.Env().Agent.Secret()

	assert.Contains(v.T(), secret, "No secret_backend_command set")
}
