package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	"github.com/stretchr/testify/assert"

	// we need some valid check in the catalog to run tests
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"
)

func TestNewConfigResolver(t *testing.T) {
	cr := newConfigResolver(nil, nil, nil)
	assert.NotNil(t, cr)
}

func TestResolveTemplate(t *testing.T) {
	ac := NewAutoConfig(nil)
	// setup the go checks loader
	l, _ := corechecks.NewGoCheckLoader()
	ac.AddLoader(l)
	tc := NewTemplateCache()
	cr := newConfigResolver(nil, ac, tc)
	tpl := check.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}
	// add the template to the cache
	tc.Set(tpl)

	// no services
	res := cr.ResolveTemplate(tpl)
	assert.Len(t, res, 0)

	service := listeners.Service{
		ADIdentifiers: []string{"redis"},
	}
	cr.processNewService(service)

	// there are no template vars but it's ok
	res = cr.ResolveTemplate(tpl)
	assert.Len(t, res, 1)
}

func TestParseTemplateVar(t *testing.T) {
	name, key := parseTemplateVar([]byte("%%host%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "", string(key))

	name, key = parseTemplateVar([]byte("%%host_0%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "0", string(key))

	name, key = parseTemplateVar([]byte("%%host 0%%"))
	assert.Equal(t, "host0", string(name))
	assert.Equal(t, "", string(key))

	name, key = parseTemplateVar([]byte("%%host_0_1%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "", string(key))
}

func TestResolve(t *testing.T) {
	cr := newConfigResolver(nil, nil, NewTemplateCache())
	service := listeners.Service{
		ADIdentifiers: []string{"redis"},
	}
	cr.processNewService(service)

	tpl := check.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}
	tpl.Instances = []check.ConfigData{check.ConfigData("host: %%host%%")}

	config, err := cr.resolve(tpl, service)
	assert.Nil(t, err)
	assert.Equal(t, "host: 127.0.0.1", string(config.Instances[0]))

	// template variable doesn't exist
	tpl.Instances = []check.ConfigData{check.ConfigData("host: %%FOO%%")}
	config, err = cr.resolve(tpl, service)
	assert.NotNil(t, err)
}
