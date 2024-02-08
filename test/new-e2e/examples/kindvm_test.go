// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type myKindVMSuite struct {
	e2e.BaseSuite[environments.KindVMHost]
}

func TestMyKindVMSuite(t *testing.T) {
	e2e.Run(t, &myKindVMSuite{}, e2e.WithProvisioner(kubernetes.Provisioner(kubernetes.WithoutFakeIntake())))
}

func (v *myKindVMSuite) TestIsAmazonLinux() {
	res, _ := v.Env().KubernetesCluster.Client().CoreV1().Pods("default").List(context.TODO(), v1.ListOptions{})
	for _, pod := range res.Items {
		fmt.Println(pod.Name)
	}
	fmt.Println(v.Env().Agent.AgentInstallName)
	assert.Equal(v.T(), res, "ami-05fab674de2157a80")
}
