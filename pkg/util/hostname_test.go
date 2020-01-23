package util

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/stretchr/testify/assert"
)

var originalConfig = config.Datadog

func restoreGlobalConfig() {
	config.Datadog = originalConfig
}

func clearHostnameCache() {
	cacheHostnameKey := cache.BuildAgentKey("hostname")
	cache.Cache.Delete(cacheHostnameKey)
}

func TestResolveSourcesWithoutState(t *testing.T) {

	liveSources := HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
	}
	stateSources := HostnameMap{}

	// live sources with endpoint flake
	resolved, change := ResolveSourcesWithState(liveSources, stateSources)

	assert.Equal(t, resolved, liveSources, "resolved state should be equal to live state")
	assert.True(t, change, "no change to persisted state expected")

	assert.Equal(t, resolved, liveSources, "resolved state should be equal to persisted state")
	assert.True(t, change, "change to persisted state expected")
}

func TestResolveSourcesWithStateAWS(t *testing.T) {

	livesources := HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
	}
	statesources := HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
		"aws":       "i-0987654321",
	}

	// live sources with endpoint flake
	resolved, change := ResolveSourcesWithState(livesources, statesources)

	assert.Equal(t, resolved, statesources, "resolved state should be equal to persisted state")
	assert.False(t, change, "no change to persisted state expected")

	// ami started on new host
	livesources["aws"] = "i-1234567890"
	resolved, change = ResolveSourcesWithState(livesources, statesources)

	assert.Equal(t, resolved, livesources, "resolved state should be equal to persisted state")
	assert.True(t, change, "change to persisted state expected")

	// configuration removed from config file (in state but not live)
	statesources[HostnameProviderConfiguration] = "custom-foo"
	resolved, change = ResolveSourcesWithState(livesources, statesources)

	assert.Equal(t, resolved, livesources, "resolved state should be equal to persisted state")
	assert.True(t, change, "change to persisted state expected")

	// configuration change in config file
	livesources[HostnameProviderConfiguration] = "custom-bar"
	resolved, change = ResolveSourcesWithState(livesources, statesources)

	assert.Equal(t, resolved, livesources, "resolved state should be equal to persisted state")
	assert.True(t, change, "change to persisted state expected")

}

func TestResolveSourcesWithStateContainer(t *testing.T) {

	// we use state left behind by container with bare-metal agent.
	liveSources := HostnameMap{
		"fqdn": "foo.domain.com",
	}
	stateSources := HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
	}

	// Live sources with endpoint flake
	resolved, change := ResolveSourcesWithState(liveSources, stateSources)

	assert.Equal(t, resolved, liveSources, "Resolved state should be equal to live state")
	assert.True(t, change, "Change to persisted state expected")

	// we mount state in container -  weird but possible.
	liveSources = HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
	}
	stateSources = HostnameMap{
		"fqdn": "foo.domain.com",
	}

	resolved, change = ResolveSourcesWithState(liveSources, stateSources)

	assert.Equal(t, resolved, liveSources, "Resolved state should be equal to live state")
	assert.True(t, change, "Change to persisted state expected")
}

func TestResolveSourcesWithStateFargate(t *testing.T) {

	// we use state left behind by container with bare-metal agent.
	liveSources := HostnameMap{
		"fargate": "",
	}
	stateSources := HostnameMap{
		"fqdn": "foo.domain.com",
	}

	// Live sources with endpoint flake
	resolved, change := ResolveSourcesWithState(liveSources, stateSources)

	assert.Equal(t, resolved, liveSources, "Resolved state should be equal to live state")
	assert.True(t, change, "Change to persisted state expected")

	// we use state left behind by container with bare-metal agent.
	liveSources = HostnameMap{
		"container": "foo",
	}
	stateSources = HostnameMap{
		"fargate": "",
	}
	expectedSources := HostnameMap{
		"fargate":   "",
		"container": "foo",
	}

	// Live sources with endpoint flake
	resolved, change = ResolveSourcesWithState(liveSources, stateSources)

	assert.Equal(t, expectedSources, resolved, "Resolved state should be equal to live state")
	assert.True(t, change, "Change to persisted state expected")
}

func TestGetHostnameData(t *testing.T) {

	defer clearHostnameCache()

	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
			"aws":  "i-0987654321",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
		}, nil
	}

	data, err := GetHostnameData()

	assert.Equal(t, "i-0987654321", data.Hostname, "Hostname resolved should be AWS instance name")
	assert.Equal(t, "aws", data.Provider, "Hostname resolved should be AWS instance name")
	assert.Nil(t, err, "No error expected")

	// With AWS flake
	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
			"aws":  "i-0987654321",
		}, nil
	}

	data, err = GetHostnameData()

	assert.Equal(t, "i-0987654321", data.Hostname, "Hostname resolved should be AWS instance name")
	assert.Equal(t, "aws", data.Provider, "Hostname resolved should be AWS instance name")
	assert.Nil(t, err, "No error expected")

}

func TestGetHostnameDataFQDN(t *testing.T) {

	defer clearHostnameCache()

	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
		}, nil
	}

	data, err := GetHostnameData()

	assert.Equal(t, "foo.domain.com", data.Hostname, "Hostname resolved should be FQDN name")
	assert.Equal(t, "fqdn", data.Provider, "Hostname provider should be FQDN")
	assert.Nil(t, err, "No error expected")

	clearHostnameCache()

	// let's assume we do not want to use the fqdn anymore
	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"os": "foo",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
			"os":   "foo",
		}, nil
	}

	data, err = GetHostnameData()

	assert.Equal(t, "foo", data.Hostname, "Hostname resolved should be OS shortname")
	assert.Equal(t, "os", data.Provider, "Hostname provider should be OS")
	assert.Nil(t, err, "No error expected")

}

func TestGetHostnameDataFargate(t *testing.T) {

	defer clearHostnameCache()

	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn":      "foo.domain.com",
			"container": "some-container",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fargate": "",
			"fqdn":    "foo.domain.com",
		}, nil
	}

	data, err := GetHostnameData()

	assert.Equal(t, "", data.Hostname, "Hostname resolved should be the Container name")
	assert.Equal(t, "fargate", data.Provider, "Hostname provider should be container")
	assert.Nil(t, err, "No error expected")
}

func TestGetHostnameDataContainerized(t *testing.T) {

	defer clearHostnameCache()

	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn":      "foo.domain.com",
			"container": "some-container",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
		}, nil
	}

	data, err := GetHostnameData()

	assert.Equal(t, "some-container", data.Hostname, "Hostname resolved should be the Container name")
	assert.Equal(t, "container", data.Provider, "Hostname provider should be container")
	assert.Nil(t, err, "No error expected")

	clearHostnameCache()

	// let's assume we had state from a container, but we're not running in
	// a container anymore.
	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn":      "foo.domain.com",
			"container": "some-container",
		}, nil
	}

	data, err = GetHostnameData()

	assert.Equal(t, "foo.domain.com", data.Hostname, "Hostname resolved should be FQDN name")
	assert.Equal(t, "fqdn", data.Provider, "Hostname provider should be FQDN")
	assert.Nil(t, err, "No error expected")

}

func TestGetHostnameDataAWS(t *testing.T) {

	defer clearHostnameCache()

	liveSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn":      "foo.domain.com",
			"container": "some-container",
			"aws":       "i-0987654321",
		}, nil
	}
	stateSourcer = func() (HostnameMap, error) {
		return HostnameMap{
			"fqdn": "foo.domain.com",
		}, nil
	}

	data, err := GetHostnameData()

	assert.Equal(t, "i-0987654321", data.Hostname, "Hostname resolved should be the AWS instance name")
	assert.Equal(t, "aws", data.Provider, "Hostname provider should be AWS")
	assert.Nil(t, err, "No error expected")

}

func TestPersistence(t *testing.T) {

	state := HostnameMap{
		"fqdn":      "foo.domain.com",
		"container": "bar",
		"aws":       "i-0987654321",
	}

	err := PersistHostnameSources(state)
	assert.Nil(t, err, "No error expected persisting to disk")

	persisted, err := GetPersistedHostnameSources()
	assert.Nil(t, err, "No error expected loading persisted state from disk")

	assert.Equal(t, state, persisted, "Persisted and loaded states expected to be equal")
}
