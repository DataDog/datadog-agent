// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package legacy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAffirmative(t *testing.T) {
	value, err := isAffirmative("yes")
	assert.Nil(t, err)
	assert.True(t, value)

	value, err = isAffirmative("True")
	assert.Nil(t, err)
	assert.True(t, value)

	value, err = isAffirmative("1")
	assert.Nil(t, err)
	assert.True(t, value)

	_, err = isAffirmative("")
	assert.NotNil(t, err)

	value, err = isAffirmative("ok")
	assert.Nil(t, err)
	assert.False(t, value)
}

func TestBuildProxySettings(t *testing.T) {
	agentConfig := make(Config)

	value, err := buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Empty(t, value)

	// malformed url
	agentConfig["proxy_host"] = "http://notanurl{}"
	_, err = buildProxySettings(agentConfig)
	assert.NotNil(t, err)

	agentConfig["proxy_host"] = "foobar.baz"

	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, "http://foobar.baz", value)

	agentConfig["proxy_port"] = "8080"

	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, "http://foobar.baz:8080", value)

	// the password alone should not be considered without an user
	agentConfig["proxy_password"] = "mypass"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, "http://foobar.baz:8080", value)

	// the user alone is ok
	agentConfig["proxy_password"] = ""
	agentConfig["proxy_user"] = "myuser"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, "http://myuser@foobar.baz:8080", value)

	agentConfig["proxy_password"] = "mypass"
	agentConfig["proxy_user"] = "myuser"
	value, err = buildProxySettings(agentConfig)
	assert.Nil(t, err)
	assert.Equal(t, "http://myuser:mypass@foobar.baz:8080", value)
}

func TestBuildSyslogURI(t *testing.T) {
	agentConfig := make(Config)

	assert.Empty(t, buildSyslogURI(agentConfig))

	agentConfig["syslog_host"] = "127.0.0.1"
	agentConfig["syslog_port"] = "1234"
	assert.Equal(t, "127.0.0.1:1234", buildSyslogURI(agentConfig))
}

func TestBuildConfigProviders(t *testing.T) {
	agentConfig := make(Config)

	// unknown config provider
	agentConfig["sd_config_backend"] = "foo"
	_, err := buildConfigProviders(agentConfig)
	assert.NotNil(t, err)

	// etcd
	agentConfig["sd_config_backend"] = "etcd"
	agentConfig["sd_backend_host"] = "127.0.0.1"
	agentConfig["sd_backend_port"] = "1234"
	agentConfig["sd_backend_username"] = "user"
	agentConfig["sd_backend_password"] = "pass"
	providers, err := buildConfigProviders(agentConfig)
	assert.Nil(t, err)
	assert.Len(t, providers, 1)
	p := providers[0]
	assert.Equal(t, "etcd", p.Name)
	assert.Equal(t, "127.0.0.1:1234", p.TemplateURL)
	assert.Equal(t, "user", p.Username)
	assert.Equal(t, "pass", p.Password)
	assert.True(t, p.Polling)
	assert.Empty(t, p.Token)

	// consul has specific settings
	agentConfig = make(Config)
	agentConfig["sd_config_backend"] = "consul"
	agentConfig["consul_token"] = "123456"
	providers, err = buildConfigProviders(agentConfig)
	assert.Nil(t, err)
	assert.Len(t, providers, 1)
	p = providers[0]
	assert.Equal(t, "consul", p.Name)
	assert.Equal(t, "123456", p.Token)
}
