// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

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
	tc.Set(tpl, "test provider")

	// no services
	res := cr.ResolveTemplate(tpl)
	assert.Len(t, res, 0)

	service := listeners.DockerService{
		ID:            "a5901276aed16ae9ea11660a41fecd674da47e8f5d8d5bce0080a611feed2be9",
		ADIdentifiers: []string{"redis"},
	}
	cr.processNewService(&service)

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
	ac := &AutoConfig{
		providerLoadedConfigs: make(map[string][]check.Config),
	}
	cr := newConfigResolver(nil, ac, NewTemplateCache())
	service := listeners.DockerService{
		ID:            "a5901276aed16ae9ea11660a41fecd674da47e8f5d8d5bce0080a611feed2be9",
		ADIdentifiers: []string{"redis"},
		Hosts:         map[string]string{"bridge": "127.0.0.1"},
		Pid:           1337,
	}
	cr.processNewService(&service)

	tpl := check.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}

	tpl.Instances = []check.ConfigData{check.ConfigData("host: %%host%%")}
	config, err := cr.resolve(tpl, &service)
	assert.Nil(t, err)
	// we must not modify original template
	assert.Equal(t, "host: %%host%%", string(tpl.Instances[0]))
	assert.Equal(t, "host: 127.0.0.1", string(config.Instances[0]))

	tpl.Instances = []check.ConfigData{check.ConfigData("pid: %%pid%%\ntags: [\"foo\"]")}
	config, err = cr.resolve(tpl, &service)
	assert.Nil(t, err)
	assert.Equal(t, "pid: 1337\ntags:\n- foo\n", string(config.Instances[0]))

	// Assert we have the two configs in the AC
	assert.Equal(t, 2, len(ac.providerLoadedConfigs[UnknownProvider]))

	// template variable doesn't exist
	tpl.Instances = []check.ConfigData{check.ConfigData("host: %%FOO%%")}
	config, err = cr.resolve(tpl, &service)
	assert.NotNil(t, err)
}

func TestGetFallbackHost(t *testing.T) {
	ip, err := getFallbackHost(map[string]string{"bridge": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bridge": "172.17.0.2"})
	assert.Equal(t, "172.17.0.2", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bar": "172.17.0.2"})
	assert.Equal(t, "", ip)
	assert.NotNil(t, err)
}
