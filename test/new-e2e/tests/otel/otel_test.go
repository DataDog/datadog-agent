// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localkubernetes contains the provisioner for the local Kubernetes based environments

package otel

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local/kubernetes"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed collector.yml
var collectorConfig string

func TestOtel(t *testing.T) {
	fmt.Println("config", collectorConfig)
	e2e.Run(t, &linuxTestSuite{}, e2e.WithProvisioner(localkubernetes.Provisioner(localkubernetes.WithAgentOptions(kubernetesagentparams.WithOTELAgent(), kubernetesagentparams.WithOTELConfig(collectorConfig)))))
}

func (s *linuxTestSuite) TestOtelAgentInstalled() {
	res, _ := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	containsOtelAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "otel-agent") {
			containsOtelAgent = true
			break
		}
	}
	assert.True(s.T(), containsOtelAgent, "Otel Agent not found")
	assert.Equal(s.T(), s.Env().Agent.NodeAgent, "otel-agent")
}
