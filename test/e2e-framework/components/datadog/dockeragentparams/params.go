// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dockeragentparams

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Params defines the parameters for the Docker Agent installation.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithImageTag]
//   - [WithRepository]
//   - [WithFullImagePath]
//   - [WithPulumiDependsOn]
//   - [WithEnvironmentVariables]
//   - [WithAgentServiceEnvVariable]
//   - [WithHostName]
//	 - [WithFakeintake]
//	 - [WithLogs]
//   - [WithExtraComposeManifest]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis

type Params struct {
	// FullImagePath is the full path of the docker agent image to use.
	// It has priority over ImageTag and Repository.
	FullImagePath string
	// ImageTag is the docker agent image tag to use.
	ImageTag string
	// Repository is the docker repository to use.
	Repository string
	// JMX is true if the JMX image is needed
	JMX bool
	// WindowsImage is true if Windows-compatible image is needed
	WindowsImage bool
	// AgentServiceEnvironment is a map of environment variables to set in the docker compose agent service's environment.
	AgentServiceEnvironment pulumi.Map
	// ExtraComposeManifests is a list of extra docker compose manifests to add beside the agent service.
	ExtraComposeManifests []docker.ComposeInlineManifest
	// EnvironmentVariables is a map of environment variables to set with the docker-compose context
	EnvironmentVariables pulumi.StringMap
	// PulumiDependsOn is a list of resources to depend on.
	PulumiDependsOn []pulumi.ResourceOption
	// FIPS is true if FIPS image is needed.
	FIPS bool
}

type Option = func(*Params) error

func NewParams(e config.Env, options ...Option) (*Params, error) {
	version := &Params{
		AgentServiceEnvironment: pulumi.Map{},
		EnvironmentVariables:    pulumi.StringMap{},
		WindowsImage:            !e.AgentLinuxOnly(),
	}

	for k, v := range e.AgentExtraEnvVars() {
		version.AgentServiceEnvironment[k] = pulumi.String(v)
	}

	return common.ApplyOption(version, options)
}

func WithImageTag(agentImageTag string) func(*Params) error {
	return func(p *Params) error {
		p.ImageTag = agentImageTag
		return nil
	}
}

func WithRepository(repository string) func(*Params) error {
	return func(p *Params) error {
		p.Repository = repository
		return nil
	}
}

// WithJMX makes the image be the one with Java installed
func WithJMX() func(*Params) error {
	return func(p *Params) error {
		p.JMX = true
		return nil
	}
}

// WithFIPS makes the image FIPS enabled
func WithFIPS() func(*Params) error {
	return func(p *Params) error {
		p.FIPS = true
		return nil
	}
}

// WithWindowsImage makes the image Windows-compatible (multi-arch with Windows)
func WithWindowsImage() func(*Params) error {
	return func(p *Params) error {
		p.WindowsImage = true
		return nil
	}
}

func WithFullImagePath(fullImagePath string) func(*Params) error {
	return func(p *Params) error {
		p.FullImagePath = fullImagePath
		return nil
	}
}

func WithPulumiDependsOn(resources ...pulumi.ResourceOption) func(*Params) error {
	return func(p *Params) error {
		p.PulumiDependsOn = append(p.PulumiDependsOn, resources...)
		return nil
	}
}

func WithEnvironmentVariables(environmentVariables pulumi.StringMap) func(*Params) error {
	return func(p *Params) error {
		p.EnvironmentVariables = environmentVariables
		return nil
	}
}

func WithTags(tags []string) func(*Params) error {
	return WithAgentServiceEnvVariable("DD_TAGS", pulumi.String(strings.Join(tags, ",")))
}

// WithAgentServiceEnvVariable set an environment variable in the docker compose agent service's environment.
func WithAgentServiceEnvVariable(key string, value pulumi.Input) func(*Params) error {
	return func(p *Params) error {
		p.AgentServiceEnvironment[key] = value
		return nil
	}
}

// WithIntake configures the agent to use the given url as intake.
// The url must be a valid Datadog intake, with a SSL valid certificate
//
// To use a fakeintake, see WithFakeintake.
//
// This option is overwritten by `WithFakeintake`.
func WithIntake(url string) func(*Params) error {
	return withIntakeHostname(pulumi.String(url), pulumi.Bool(false))
}

// WithFakeintake installs the fake intake and configures the Agent to use it.
//
// This option is overwritten by `WithIntakeHostname`.
func WithFakeintake(fakeintake *fakeintake.Fakeintake) func(*Params) error {
	shouldSkipSSLValidation := fakeintake.Scheme.ApplyT(func(scheme string) bool { return scheme == "http" }).(pulumi.BoolInput)
	return func(p *Params) error {
		p.PulumiDependsOn = append(p.PulumiDependsOn, utils.PulumiDependsOn(fakeintake))
		return withIntakeHostname(fakeintake.URL, shouldSkipSSLValidation)(p)
	}
}

func withIntakeHostname(url pulumi.StringInput, shouldSkipSSLValidation pulumi.BoolInput) func(*Params) error {
	return func(p *Params) error {
		envVars := pulumi.Map{
			"DD_DD_URL":                                  pulumi.Sprintf("%s", url),
			"DD_PROCESS_CONFIG_PROCESS_DD_URL":           pulumi.Sprintf("%s", url),
			"DD_APM_DD_URL":                              pulumi.Sprintf("%s", url),
			"DD_SKIP_SSL_VALIDATION":                     shouldSkipSSLValidation,
			"DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION":  shouldSkipSSLValidation,
			"DD_LOGS_CONFIG_FORCE_USE_HTTP":              pulumi.Bool(true), // Force the use of HTTP/HTTPS rather than switching to TCP
			"DD_LOGS_CONFIG_LOGS_DD_URL":                 pulumi.Sprintf("%s", url),
			"DD_LOGS_CONFIG_LOGS_NO_SSL":                 shouldSkipSSLValidation,
			"DD_SERVICE_DISCOVERY_FORWARDER_LOGS_DD_URL": pulumi.Sprintf("%s", url),
		}
		for key, value := range envVars {
			if err := WithAgentServiceEnvVariable(key, value)(p); err != nil {
				return err
			}
		}
		return nil
	}
}

type additionalLogEndpointInput struct {
	Hostname   string `json:"host"`
	APIKey     string `json:"api_key,omitempty"`
	Port       string `json:"port,omitempty"`
	IsReliable bool   `json:"is_reliable,omitempty"`
}

func WithAdditionalFakeintake(fakeintake *fakeintake.Fakeintake) func(*Params) error {
	additionalEndpointsContentInput := fakeintake.URL.ToStringOutput().ApplyT(func(url string) (string, error) {
		endpoints := map[string][]string{
			fmt.Sprintf("%s", url): {"00000000000000000000000000000000"},
		}
		jsonContent, err := json.Marshal(endpoints)
		return string(jsonContent), err
	}).(pulumi.StringOutput)

	additionalLogsEndpointsContentInput := fakeintake.Host.ToStringOutput().ApplyT(func(host string) (string, error) {
		endpoints := []additionalLogEndpointInput{
			{
				Hostname: host,
			},
		}
		jsonContent, err := json.Marshal(endpoints)
		return string(jsonContent), err
	}).(pulumi.StringOutput)

	// fakeintake without LB does not have a valid SSL certificate and accepts http only
	shouldEnforceHTTPInputAndSkipSSL := fakeintake.Scheme.ApplyT(func(scheme string) bool { return scheme == "http" }).(pulumi.BoolInput)

	return func(p *Params) error {
		logsEnvVars := pulumi.Map{
			"DD_ADDITIONAL_ENDPOINTS":                   additionalEndpointsContentInput,
			"DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS":       additionalLogsEndpointsContentInput,
			"DD_SKIP_SSL_VALIDATION":                    shouldEnforceHTTPInputAndSkipSSL,
			"DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION": shouldEnforceHTTPInputAndSkipSSL,
			"DD_LOGS_CONFIG_LOGS_NO_SSL":                shouldEnforceHTTPInputAndSkipSSL,
			"DD_LOGS_CONFIG_FORCE_USE_HTTP":             pulumi.Bool(true), // Force the use of HTTP/HTTPS rather than switching to TCP
		}
		for key, value := range logsEnvVars {
			if err := WithAgentServiceEnvVariable(key, value)(p); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithLogs enables the log agent
func WithLogs() func(*Params) error {
	return WithAgentServiceEnvVariable("DD_LOGS_ENABLED", pulumi.String("true"))
}

// WithExtraComposeManifest adds a docker.ComposeInlineManifest
func WithExtraComposeManifest(name string, content pulumi.StringInput) func(*Params) error {
	return WithExtraComposeInlineManifest(docker.ComposeInlineManifest{
		Name:    name,
		Content: content,
	})
}

// WithExtraComposeInlineManifest adds extra docker.ComposeInlineManifest
func WithExtraComposeInlineManifest(cpms ...docker.ComposeInlineManifest) func(*Params) error {
	return func(p *Params) error {
		p.ExtraComposeManifests = lo.Uniq(append(p.ExtraComposeManifests, cpms...))
		return nil
	}
}
