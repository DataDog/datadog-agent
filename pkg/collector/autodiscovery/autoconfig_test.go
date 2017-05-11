package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockProvider struct {
	CollectCounter int
}

func (p *MockProvider) Collect() ([]check.Config, error) {
	p.CollectCounter++
	return []check.Config{}, nil
}

type MockProvider2 struct {
	MockProvider
}

type MockLoader struct{}

func (l *MockLoader) Load(config check.Config) ([]check.Check, error) { return []check.Check{}, nil }

func TestAddProvider(t *testing.T) {
	ac := NewAutoConfig(nil)
	assert.Len(t, ac.providers, 0)
	mp := &MockProvider{}
	ac.AddProvider(mp, false)
	ac.AddProvider(mp, false) // this should be a noop
	ac.AddProvider(&MockProvider2{}, true)
	require.Len(t, ac.providers, 2)
	assert.Equal(t, 1, mp.CollectCounter)
	assert.False(t, ac.providers[0].poll)
	assert.True(t, ac.providers[1].poll)
}

func TestAddLoader(t *testing.T) {
	ac := NewAutoConfig(nil)
	assert.Len(t, ac.loaders, 0)
	ac.AddLoader(&MockLoader{})
	ac.AddLoader(&MockLoader{}) // noop
	assert.Len(t, ac.loaders, 1)
}

func TestContains(t *testing.T) {
	c1 := check.Config{Name: "bar"}
	c2 := check.Config{Name: "foo"}
	pd := providerDescriptor{}
	pd.configs = append(pd.configs, c1)
	assert.True(t, pd.contains(&c1))
	assert.False(t, pd.contains(&c2))
}
