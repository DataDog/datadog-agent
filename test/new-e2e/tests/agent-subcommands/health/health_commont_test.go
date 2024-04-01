// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
)

type baseHealthSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSubcommandSuite(t *testing.T) {
	e2e.Run(t, &baseHealthSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// section contains the content status of a specific section (e.g. Forwarder)
func (v *baseHealthSuite) TestDefaultInstallHealthy() {
	interval := 1 * time.Second

	var output string
	var err error
	err = backoff.Retry(func() error {
		output, err = v.Env().Agent.Client.Health()
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(15)))

	assert.NoError(v.T(), err)
	assert.Contains(v.T(), output, "Agent health: PASS")
}
