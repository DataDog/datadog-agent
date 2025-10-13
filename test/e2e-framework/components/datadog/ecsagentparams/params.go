package ecsagentparams

import (
	"fmt"
	"strings"

	"github.com/DataDog/test-infra-definitions/common"
)

// Params defines the parameters for the ECS Agent installation.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithAgentServiceEnvVariable]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis

type Params struct {
	// AgentServiceEnvironment is a map of environment variables to set in the docker compose agent service's environment.
	AgentServiceEnvironment map[string]string
	NetworkMode             string
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{
		AgentServiceEnvironment: make(map[string]string),
		NetworkMode:             "bridge",
	}
	return common.ApplyOption(version, options)
}

// WithAgentServiceEnvVariable set an environment variable in the ECS compose agent service's environment.
func WithAgentServiceEnvVariable(key string, value string) func(*Params) error {
	return func(p *Params) error {
		p.AgentServiceEnvironment[key] = value
		return nil
	}
}

// WithNetworkMode set the network mode used by the Daemon agent
func WithNetworkMode(mode string) func(*Params) error {
	return func(p *Params) error {
		switch mode {
		case "host":
		case "bridge":
		case "awsvpc":
		default:
			return fmt.Errorf("invalid network mode '%s'", mode)
		}
		p.NetworkMode = mode
		return nil
	}
}

func WithTags(tags []string) func(*Params) error {
	return WithAgentServiceEnvVariable("DD_TAGS", strings.Join(tags, ","))
}
