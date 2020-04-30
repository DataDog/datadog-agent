package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	agent "github.com/DataDog/datadog-agent/pkg/config"
)

var (
	ErrUnnamedPolicy = errors.New("unnamed policy")
)

type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

type Config struct {
	Debug            bool
	PerfMapPageCount int
	Policies         []Policy
	MaxKernelFilters int
}

func NewConfig() (*Config, error) {
	c := &Config{
		PerfMapPageCount: agent.Datadog.GetInt("security_agent.perf_map_page_count"),
		Debug:            agent.Datadog.GetBool("security_agent.debug"),
		MaxKernelFilters: agent.Datadog.GetInt("security_agent.max_kernel_filters"),
	}

	policies, ok := agent.Datadog.Get("security_agent.policies").([]interface{})
	if !ok {
		return nil, errors.New("policies must be a list of policy definitions")
	}

	for _, p := range policies {
		var policy Policy
		if err := mapstructure.Decode(p, &policy); err != nil {
			return nil, errors.Wrap(err, "invalid policy definition")
		}

		if policy.Name == "" {
			return nil, ErrUnnamedPolicy
		}

		c.Policies = append(c.Policies, policy)
	}

	return c, nil
}
