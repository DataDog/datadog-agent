// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const (
	awsPublicRegistry     = "669783387624.dkr.ecr.us-east-1.amazonaws.com/ecr-public/datadog"
	awsDockerhubMirror    = "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub"
	gcpPublicRegistry     = "gcr.io/datadoghq"
	publicDockerhubMirror = "registry-1.docker.io"
)

// imageTestEnv is a config.Env whose only real behaviour is the registry accessors and
// the pulumi context; every other accessor returns the zero value, which puts image
// resolution on its default (no pipeline, no explicit image) path.
type imageTestEnv struct {
	config.Env
	ctx              *pulumi.Context
	publicRegistry   string
	dockerhubMirror  string
	agentFIPS        bool
	agentLinuxOnly   bool
	agentFullImage   string
	clusterFullImage string
}

func (e *imageTestEnv) Ctx() *pulumi.Context              { return e.ctx }
func (e *imageTestEnv) DatadogPublicRegistry() string     { return e.publicRegistry }
func (e *imageTestEnv) InternalDockerhubMirror() string   { return e.dockerhubMirror }
func (e *imageTestEnv) InternalRegistry() string          { return "internal.example.com" }
func (e *imageTestEnv) PipelineID() string                { return "" }
func (e *imageTestEnv) CommitSHA() string                 { return "" }
func (e *imageTestEnv) AgentFullImagePath() string        { return e.agentFullImage }
func (e *imageTestEnv) ClusterAgentFullImagePath() string { return e.clusterFullImage }
func (e *imageTestEnv) AgentFIPS() bool                   { return e.agentFIPS }
func (e *imageTestEnv) AgentLinuxOnly() bool              { return e.agentLinuxOnly }
func (e *imageTestEnv) MajorVersion() string              { return "" }
func (e *imageTestEnv) AgentVersion() string              { return "" }
func (e *imageTestEnv) ClusterAgentVersion() string       { return "" }

type noopMocks struct{}

func (noopMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (noopMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "-id", args.Inputs, nil
}

// withEnv runs resolve against a pulumi context, since image resolution logs through it.
func withEnv(t *testing.T, env *imageTestEnv, resolve func(config.Env) string) string {
	t.Helper()
	var got string
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		env.ctx = ctx
		got = resolve(env)
		return nil
	}, pulumi.WithMocks("project", "stack", noopMocks{}))
	if err != nil {
		t.Fatalf("pulumi.RunErr() error = %v", err)
	}
	return got
}

func TestDockerAgentFullImagePathDefaults(t *testing.T) {
	tests := []struct {
		name     string
		env      imageTestEnv
		jmx      bool
		otel     bool
		fips     bool
		expected string
	}{
		{
			name:     "aws pulls the agent through the pull-through cache",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror},
			expected: awsPublicRegistry + "/agent:latest",
		},
		{
			name:     "gcp pulls the agent from the public registry",
			env:      imageTestEnv{publicRegistry: gcpPublicRegistry, dockerhubMirror: publicDockerhubMirror},
			expected: gcpPublicRegistry + "/agent:latest",
		},
		{
			// The dev images are only published to Docker Hub, so they go through the
			// Docker Hub mirror rather than the public Datadog registry.
			name:     "fips uses the dev repository behind the dockerhub mirror",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror},
			fips:     true,
			expected: awsDockerhubMirror + "/datadog/agent-dev:main-fips",
		},
		{
			name:     "otel uses the dev repository behind the dockerhub mirror",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror},
			otel:     true,
			expected: awsDockerhubMirror + "/datadog/agent-dev:nightly-full-main-jmx",
		},
		{
			name:     "an explicit full image path wins over the registry",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror, agentFullImage: "example.com/agent:pinned"},
			expected: "example.com/agent:pinned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			got := withEnv(t, &env, func(e config.Env) string {
				return dockerAgentFullImagePath(e, "", "", tt.otel, tt.fips, tt.jmx, false)
			})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDockerClusterAgentFullImagePathDefaults(t *testing.T) {
	tests := []struct {
		name     string
		env      imageTestEnv
		fips     bool
		expected string
	}{
		{
			name:     "aws pulls the cluster agent through the pull-through cache",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror},
			expected: awsPublicRegistry + "/cluster-agent:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			got := withEnv(t, &env, func(e config.Env) string {
				return dockerClusterAgentFullImagePath(e, "", "", tt.fips)
			})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDockerOTelAgentGatewayFullImagePathDefaults(t *testing.T) {
	tests := []struct {
		name     string
		env      imageTestEnv
		expected string
	}{
		{
			name:     "aws pulls the gateway through the pull-through cache",
			env:      imageTestEnv{publicRegistry: awsPublicRegistry, dockerhubMirror: awsDockerhubMirror},
			expected: awsPublicRegistry + "/ddot-collector:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			got := withEnv(t, &env, func(e config.Env) string {
				return dockerOTelAgentGatewayFullImagePath(e, "", "")
			})
			assert.Equal(t, tt.expected, got)
		})
	}
}
