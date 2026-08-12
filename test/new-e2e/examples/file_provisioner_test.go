// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// envDescriptorPath is the path to a JSON file that describes a pre-existing
// Kubernetes environment (cluster kubeconfig, fakeintake address, …).
// Produce one with the kind_nopulumi helper or by exporting a live Pulumi stack.
var envDescriptorPath = flag.String("env-descriptor", "", "path to the environment JSON descriptor file")

type fileProvisionerSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestFileProvisionerSuite demonstrates using [provisioners.TypedFileProvisioner]
// to load a pre-existing Kubernetes environment from a JSON file at runtime,
// without spinning up any Pulumi infrastructure.
//
// Run with:
//
//	go test ./examples/ -run TestFileProvisionerSuite -env-descriptor /path/to/env.json
func TestFileProvisionerSuite(t *testing.T) {
	if *envDescriptorPath == "" {
		t.Skip("no --env-descriptor provided; pass -env-descriptor=/path/to/env.json to run")
	}

	absPath, err := filepath.Abs(*envDescriptorPath)
	if err != nil {
		t.Fatalf("invalid --env-descriptor path: %v", err)
	}

	e2e.Run(t, &fileProvisionerSuite{},
		e2e.WithProvisioner(provisioners.NewTypedFileProvisioner[environments.Kubernetes]("",
			os.DirFS(filepath.Dir(absPath)),
		)),
	)
}

func (s *fileProvisionerSuite) TestKubernetesClusterIsLoaded() {
	kc := s.Env().KubernetesCluster

	s.Require().NotNil(kc)
	s.Assert().NotEmpty(kc.ClusterName)
	s.Assert().NotEmpty(kc.KubeConfig)
	s.Assert().NotNil(kc.KubernetesClient)
}

func (s *fileProvisionerSuite) TestFakeIntakeIsLoaded() {
	fi := s.Env().FakeIntake

	s.Require().NotNil(fi)
	s.Assert().NotEmpty(fi.URL)
	s.Assert().NotNil(fi.Client())
}

func (s *fileProvisionerSuite) TestAgentIsNil() {
	// Agent was not part of the descriptor — TypedFileProvisioner sets it to nil.
	s.Assert().Nil(s.Env().Agent)
}
