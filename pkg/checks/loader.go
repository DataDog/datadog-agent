package checks

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/checks/system"
	"github.com/DataDog/datadog-agent/pkg/loader"
)

const catalog = map[string]Check{
	"memory": system.MemoryCheck,
}

// GoCheckLoader is a specific loader for checks living in this package
type GoCheckLoader struct {
}

func NewGoCheckLoader() *GoCheckLoader {
	return &GoCheckLoader{}
}

func (gl *GoCheckLoader) Load(config loader.CheckConfig) ([]checks.Check, error) {
	checks := []checks.Check{}

	c, found := catalog[config.Name]
	if !found {
		return checks, fmt.Errorf("Check %s not found", config.Name)
	}

	// Get an AgentCheck for each configuration instance and add it to the registry
	instances, found := config.Data["instances"]
	if !found {
		return checks, errors.New("`instances` keyword not found in configuration data")
	}

	instancesList, _ := instances.([]interface{})
	for _, instanceMap := range instancesList {
		newCheck := c{}
		c.Configure()
		checks = append(checks, c)
	}

	return checks, nil
}
