package cloudfoundry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestHostAliasDisable(t *testing.T) {
	mockConfig := config.Mock()

	mockConfig.Set("cloud_foundry", false)
	mockConfig.Set("bosh_id", "ID_CF")

	aliases, err := GetHostAliases()
	assert.Nil(t, err)
	assert.Nil(t, aliases)
}

func TestHostAlias(t *testing.T) {
	defer func() { getFqdn = util.Fqdn }()
	mockConfig := config.Mock()

	mockConfig.Set("cloud_foundry", true)
	mockConfig.Set("bosh_id", "ID_CF")
	mockConfig.Set("cf_os_hostname_aliasing", false)

	aliases, err := GetHostAliases()
	assert.Nil(t, err)
	assert.Equal(t, []string{"ID_CF"}, aliases)

	mockConfig.Set("cf_os_hostname_aliasing", true)
	// mock Fqdn returning hostname unchanged
	getFqdn = func(hostname string) string {
		return hostname
	}
	aliases, err = GetHostAliases()
	assert.Nil(t, err)

	hostname, _ := os.Hostname()

	assert.Len(t, aliases, 2)
	assert.Contains(t, aliases, "ID_CF")
	assert.Contains(t, aliases, hostname)

	// mock Fqdn returning something different
	getFqdn = func(hostname string) string {
		return hostname + "suffix"
	}
	aliases, err = GetHostAliases()
	assert.Nil(t, err)
	assert.Len(t, aliases, 3)
	assert.Contains(t, aliases, "ID_CF")
	assert.Contains(t, aliases, hostname)
	assert.Contains(t, aliases, hostname+"suffix")

}

func TestHostAliasDefault(t *testing.T) {
	mockConfig := config.Mock()

	mockConfig.Set("cloud_foundry", true)
	mockConfig.Set("bosh_id", nil)
	mockConfig.Set("cf_os_hostname_aliasing", nil)

	aliases, err := GetHostAliases()
	assert.Nil(t, err)

	hostname, _ := os.Hostname()
	assert.Equal(t, []string{util.Fqdn(hostname)}, aliases)
}
