// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/assert"
)

type baseSecretSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (v *baseSecretSuite) TestAgentSecretNotEnabledByDefault() {
	secret := v.Env().Agent.Client.Secret()

	assert.Contains(v.T(), secret, "No secret_backend_command set")
}
