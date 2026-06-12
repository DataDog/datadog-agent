// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockeragent provides functions to install and configure the Datadog
// Agent as a Docker container on a remote host via SSH, without relying on
// Pulumi. The agent is deployed via a docker-compose file written to the remote
// host.
package dockeragent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// testContext adapts *testing.T to common.Context for agent client initialization.
type testContext struct{ t *testing.T }

func (c *testContext) T() *testing.T            { return c.t }
func (c *testContext) SessionOutputDir() string { return "" }

const (
	agentContainerName     = "datadog-agent"
	composeTmpDir          = "/tmp/datadog-agent-compose"
	composeFile            = composeTmpDir + "/docker-compose.yml"
	defaultAgentImageRepo  = "gcr.io/datadoghq/agent"
	defaultAgentMajorTag   = "7"
	internalAgentQASubpath = "agent-qa"
)

// Option configures the docker agent installer.
type Option func(*Params)

// Params holds the Pulumi-free configuration for the docker agent installer.
type Params struct {
	// FullImagePath overrides all other image fields when set.
	FullImagePath string
	// Repository is the docker image repository (e.g. "gcr.io/datadoghq/agent").
	Repository string
	// ImageTag overrides the resolved tag.
	ImageTag string
	// FIPS requests the FIPS image variant.
	FIPS bool
	// JMX requests the JMX image variant.
	JMX bool
	// EnvVars are additional environment variables to inject into the agent container.
	EnvVars map[string]string
}

// WithFullImagePath sets a fully-qualified image path, overriding version resolution.
func WithFullImagePath(path string) Option {
	return func(p *Params) { p.FullImagePath = path }
}

// WithRepository sets the docker repository.
func WithRepository(repo string) Option {
	return func(p *Params) { p.Repository = repo }
}

// WithImageTag sets a specific image tag.
func WithImageTag(tag string) Option {
	return func(p *Params) { p.ImageTag = tag }
}

// WithFIPS requests the FIPS image variant.
func WithFIPS() Option {
	return func(p *Params) { p.FIPS = true }
}

// WithJMX requests the JMX image variant.
func WithJMX() Option {
	return func(p *Params) { p.JMX = true }
}

// WithEnvVar adds a single environment variable to the agent container.
func WithEnvVar(key, value string) Option {
	return func(p *Params) {
		if p.EnvVars == nil {
			p.EnvVars = make(map[string]string)
		}
		p.EnvVars[key] = value
	}
}

// Install deploys the Datadog Agent as a Docker container on the remote host
// and populates env.Agent with an initialized agent client. The Docker daemon
// and docker-compose must already be running on the host (provisioned by Pulumi).
//
// Usage in SetupSuite:
//
//	dockeragent.Install(s.T(), s.Env())
func Install(t *testing.T, env *environments.DockerHost, opts ...Option) {
	t.Helper()
	require.NotNil(t, env.RemoteHost, "dockeragent.Install: RemoteHost is nil, infrastructure must be provisioned first")

	p := &Params{}
	for _, o := range opts {
		o(p)
	}

	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err, "failed to get API key")
	apiKey = strings.TrimSpace(apiKey)

	imagePath := resolveImagePath(t, p)
	fakeintakeURL := fakeintakeEnvVars(env)
	composeYAML := buildComposeYAML(imagePath, apiKey, fakeintakeURL, p.EnvVars)

	// Write compose file and start the agent container.
	env.RemoteHost.MustExecute(fmt.Sprintf("mkdir -p %s", composeTmpDir))
	env.RemoteHost.MustExecute(fmt.Sprintf("cat > %s << 'COMPOSEOF'\n%s\nCOMPOSEOF", composeFile, composeYAML))
	env.RemoteHost.MustExecute(fmt.Sprintf("docker-compose -f %s up -d --wait 2>&1 || docker compose -f %s up -d --wait 2>&1", composeFile, composeFile))

	// Populate the DockerAgent component so tests can call env.Agent.Client.
	env.Agent = &components.DockerAgent{}
	env.Agent.DockerAgentOutput.DockerManager.Host = env.RemoteHost.HostOutput
	env.Agent.DockerAgentOutput.ContainerName = agentContainerName
	err = env.Agent.Init(&testContext{t: t})
	require.NoError(t, err, "failed to initialize docker agent client")
}

// resolveImagePath determines the docker image path from Params and the runner profile.
func resolveImagePath(t *testing.T, p *Params) string {
	t.Helper()

	if p.FullImagePath != "" {
		return p.FullImagePath
	}

	profile := runner.GetProfile()

	// Pipeline image: use internal registry when pipelineID + commitSHA are set.
	pipelineID, _ := profile.ParamStore().GetWithDefault(parameters.PipelineID, "")
	commitSHA, _ := profile.ParamStore().GetWithDefault(parameters.CommitSHA, "")
	if pipelineID != "" && commitSHA != "" && p.ImageTag == "" {
		tag := fmt.Sprintf("%s-%s", pipelineID, commitSHA)
		if p.FIPS {
			tag += "-fips"
		}
		if p.JMX {
			tag += "-jmx"
		}
		registry := runner.InternalRegistry(runner.CloudAWS) // default to AWS for docker
		return fmt.Sprintf("%s/%s:%s", registry, internalAgentQASubpath, tag)
	}

	repo := p.Repository
	if repo == "" {
		repo = defaultAgentImageRepo
	}

	tag := p.ImageTag
	if tag == "" {
		major, _ := profile.ParamStore().GetWithDefault(parameters.MajorVersion, defaultAgentMajorTag)
		tag = major
		if p.JMX {
			tag += "-jmx"
		}
	}

	return fmt.Sprintf("%s:%s", repo, tag)
}

// fakeintakeEnvVars returns the environment variables needed to point the agent
// at the fakeintake, or an empty map if no fakeintake is provisioned.
func fakeintakeEnvVars(env *environments.DockerHost) map[string]string {
	if env.FakeIntake == nil {
		return nil
	}
	url := env.FakeIntake.URL
	if url == "" {
		return nil
	}
	skipSSL := "false"
	if strings.HasPrefix(url, "http://") {
		skipSSL = "true"
	}
	return map[string]string{
		"DD_DD_URL":                  url,
		"DD_SKIP_SSL_VALIDATION":     skipSSL,
		"DD_PROCESS_DD_URL":          url,
		"DD_APM_DD_URL":              url,
		"DD_LOGS_CONFIG_LOGS_DD_URL": url + ":443",
		"DD_LOGS_CONFIG_USE_HTTP":    "true",
		"DD_LOGS_CONFIG_LOGS_NO_SSL": skipSSL,
	}
}

type composeManifest struct {
	Version  string                    `yaml:"version"`
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Pid           string            `yaml:"pid,omitempty"`
	Privileged    bool              `yaml:"privileged,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name"`
	Volumes       []string          `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
}

func buildComposeYAML(imagePath, apiKey string, fakeintakeVars map[string]string, extraEnvVars map[string]string) string {
	env := map[string]string{
		"DD_API_KEY": apiKey,
		"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "true",
	}
	for k, v := range fakeintakeVars {
		env[k] = v
	}
	for k, v := range extraEnvVars {
		env[k] = v
	}

	// Enable privileged mode when system probe env vars are present.
	privileged := false
	for k := range env {
		if strings.HasPrefix(k, "DD_SYSTEM_PROBE_") {
			privileged = true
			break
		}
	}

	manifest := composeManifest{
		Version: "3.9",
		Services: map[string]composeService{
			"agent": {
				Privileged:    privileged,
				Image:         imagePath,
				ContainerName: agentContainerName,
				Volumes: []string{
					"/var/run/docker.sock:/var/run/docker.sock",
					"/proc/:/host/proc",
					"/sys/fs/cgroup/:/host/sys/fs/cgroup",
					"/var/run/datadog:/var/run/datadog",
					"/sys/kernel/tracing:/sys/kernel/tracing",
				},
				Environment: env,
				Pid:         "host",
				Ports:       []string{"8125:8125/udp", "8126:8126/tcp"},
			},
		},
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		panic(fmt.Sprintf("dockeragent: failed to marshal compose YAML: %v", err))
	}
	return string(data)
}
