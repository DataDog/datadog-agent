package checks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/check"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("datadog-agent")

var Catalog = make(map[string]check.Check)

func RegisterCheck(name string, c check.Check) {
	Catalog[name] = c
}

// GoCheckLoader is a specific loader for checks living in this package
type GoCheckLoader struct {
}

func NewGoCheckLoader() *GoCheckLoader {
	return &GoCheckLoader{}
}

func (gl *GoCheckLoader) Load(config check.Config) ([]check.Check, error) {
	checks := []check.Check{}

	c, found := Catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		log.Warning(msg)
		return checks, fmt.Errorf(msg)
	}

	for _, instance := range config.Instances {
		newCheck := c
		newCheck.Configure(instance)
		checks = append(checks, newCheck)
	}

	return checks, nil
}
