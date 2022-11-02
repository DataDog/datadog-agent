// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestHostAliasDisable(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)

	mockConfig.Set("cloud_foundry", false)
	mockConfig.Set("bosh_id", "ID_CF")

	aliases, err := GetHostAliases(ctx)
	assert.Nil(t, err)
	assert.Nil(t, aliases)
}

func TestHostAlias(t *testing.T) {
	ctx := context.Background()
	defer func() { getFqdn = util.Fqdn }()
	mockConfig := config.Mock(t)

	mockConfig.Set("cloud_foundry", true)
	mockConfig.Set("bosh_id", "ID_CF")
	mockConfig.Set("cf_os_hostname_aliasing", false)

	aliases, err := GetHostAliases(ctx)
	assert.Nil(t, err)
	assert.Equal(t, []string{"ID_CF"}, aliases)

	mockConfig.Set("cf_os_hostname_aliasing", true)
	// mock Fqdn returning hostname unchanged
	getFqdn = func(hostname string) string {
		return hostname
	}
	aliases, err = GetHostAliases(ctx)
	assert.Nil(t, err)

	hostname, _ := os.Hostname()

	assert.Len(t, aliases, 2)
	assert.Contains(t, aliases, "ID_CF")
	assert.Contains(t, aliases, hostname)

	// mock Fqdn returning something different
	getFqdn = func(hostname string) string {
		return hostname + "suffix"
	}
	aliases, err = GetHostAliases(ctx)
	assert.Nil(t, err)
	assert.Len(t, aliases, 3)
	assert.Contains(t, aliases, "ID_CF")
	assert.Contains(t, aliases, hostname)
	assert.Contains(t, aliases, hostname+"suffix")

}

func TestHostAliasDefault(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockHostname := "hostname"

	// mock getFqdn to avoid flakes in CI runners
	getFqdn = func(hostname string) string {
		return mockHostname
	}

	mockConfig.Set("cloud_foundry", true)
	mockConfig.Set("bosh_id", nil)
	mockConfig.Set("cf_os_hostname_aliasing", nil)

	aliases, err := GetHostAliases(ctx)
	assert.Nil(t, err)

	assert.Equal(t, []string{mockHostname}, aliases)
}
