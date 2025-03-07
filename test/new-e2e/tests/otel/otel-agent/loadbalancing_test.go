// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type loadBalancingTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/loadbalancing.yml
var loadBalancingConfig string

func TestOTelAgentLoadBalancing(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &loadBalancingTestSuite{}, e2e.WithProvisioner(localkubernetes.Provisioner(localkubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(loadBalancingConfig)))))
}

func (s *loadBalancingTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s, false, "calendar-rest-go-1")
	utils.TestCalendarApp(s, false, "calendar-rest-go-2")
	utils.TestCalendarApp(s, false, "calendar-rest-go-3")
	utils.TestCalendarApp(s, false, "calendar-rest-go-4")
}

func (s *loadBalancingTestSuite) TestLoadBalancing() {
	utils.TestLoadBalancing(s)
}
