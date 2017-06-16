package corecheck

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
)

type checkFactory func() check.Check

// Catalog keeps track of Go checks by name
var catalog = make(map[string]checkFactory)

// RegisterCheck adds a check to the catalog
func RegisterCheck(name string, c checkFactory) {
	catalog[name] = c
}

// GoCheckLoader is a specific loader for checks living in this package
type GoCheckLoader struct{}

// NewGoCheckLoader creates a loader for go checks
func NewGoCheckLoader() *GoCheckLoader {
	return &GoCheckLoader{}
}

// Load returns a list of checks, one for every configuration instance found in `config`
func (gl *GoCheckLoader) Load(config check.Config) ([]check.Check, error) {
	checks := []check.Check{}

	factory, found := catalog[config.Name]
	if !found {
		msg := fmt.Sprintf("Check %s not found in Catalog", config.Name)
		return checks, fmt.Errorf(msg)
	}

	for _, instance := range config.Instances {
		newCheck := factory()
		if err := newCheck.Configure(instance, config.InitConfig); err != nil {
			log.Errorf("core.loader: could not configure check %s: %s", newCheck, err)
			continue
		}
		newCheck.InitSender()
		checks = append(checks, newCheck)
	}

	return checks, nil
}

func (gl *GoCheckLoader) String() string {
	return "Core Check Loader"
}
