// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

//nolint:misspell // misspell only handles english
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
