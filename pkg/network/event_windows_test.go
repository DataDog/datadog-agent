package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var englishOut = `
Protocol tcp Dynamic Port Range
---------------------------------
Start Port      : 49152
Number of Ports : 16384`

var frenchOut = `
Plage de ports dynamique du protocole tcp
---------------------------------
Port de démarrage   : 49152
Nombre de ports     : 16384
`

func TestNetshParse(t *testing.T) {
	t.Run("english", func(t *testing.T) {
		low, hi, err := parseNetshOutput(englishOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
	t.Run("french", func(t *testing.T) {
		low, hi, err := parseNetshOutput(frenchOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
}
