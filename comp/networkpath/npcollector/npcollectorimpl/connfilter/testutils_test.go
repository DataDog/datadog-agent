package connfilter

import (
	"errors"
	"testing"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

func getConnFilter(t *testing.T, configString string, ddSite string) (*ConnFilter, error) {
	var configs []ConnFilterConfig

	cfg := configComponent.NewMockFromYAML(t, configString)
	err := structure.UnmarshalKey(cfg, "filters", &configs)
	if err != nil {
		return nil, err
	}
	connFilter, errs := NewConnFilter(configs, ddSite)
	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return connFilter, err
}
