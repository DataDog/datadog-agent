package meta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProdDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseProdTUFVersion(t *testing.T) {
	v, err := parseRootVersion(prodRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingDirectorVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootDirector)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}

func TestParseStagingTUFVersion(t *testing.T) {
	v, err := parseRootVersion(stagingRootConfig)
	require.NoError(t, err)
	require.Greater(t, v, uint64(0))
}
