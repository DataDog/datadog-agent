package loaders

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type LoaderOne struct{}

func (lo LoaderOne) Load(config check.Config) ([]check.Check, error) { return nil, nil }

type LoaderTwo struct{}

func (lt LoaderTwo) Load(config check.Config) ([]check.Check, error) { return nil, nil }

func TestLoaderCatalog(t *testing.T) {
	l1 := LoaderOne{}
	factory1 := func() check.Loader { return l1 }
	l2 := LoaderTwo{}
	factory2 := func() check.Loader { return l2 }

	RegisterLoader(20, factory1)
	RegisterLoader(10, factory2)

	require.Len(t, LoaderCatalog(), 2)
	assert.Equal(t, l1, LoaderCatalog()[1])
	assert.Equal(t, l2, LoaderCatalog()[0])
}
