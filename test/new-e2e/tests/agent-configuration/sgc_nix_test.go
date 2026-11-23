// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	_ "embed"
	"testing"

	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type sgcLinuxRuntimeSuite struct {
	sgcBaseRuntimeSuite
}

//go:embed sgc-embedded/fixtures/secrets.yaml
var embeddedSecretFileYAML string

func TestSGCLinuxRuntimeSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &sgcLinuxRuntimeSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithEC2InstanceOptions(scenec2.WithInternetAccess())))))
}
