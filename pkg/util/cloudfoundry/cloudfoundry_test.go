package cloudfoudry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestHostAliasDisable(t *testing.T) {
	config.Datadog.Set("cloud_foundry", false)
	config.Datadog.Set("bosh_id", "ID_CF")

	alias, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Empty(t, alias)
}

func TestHostAlias(t *testing.T) {
	config.Datadog.Set("cloud_foundry", true)
	config.Datadog.Set("bosh_id", "ID_CF")

	alias, err := GetHostAlias()
	assert.Nil(t, err)
	assert.Equal(t, "ID_CF", alias)
}

func TestHostAliasDefault(t *testing.T) {
	config.Datadog.Set("cloud_foundry", true)
	config.Datadog.Set("bosh_id", nil)

	alias, err := GetHostAlias()
	assert.Nil(t, err)

	hostname, _ := os.Hostname()
	assert.Equal(t, util.Fqdn(hostname), alias)
}
