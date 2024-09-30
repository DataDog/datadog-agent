/* SPDX-License-Identifier: BSD-2-Clause */

package results

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixUsecMarshalJSON(t *testing.T) {
	// 10 seconds
	um := UnixUsec(time.Unix(10, 0))
	b, err := um.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, string("10.000000"), string(b))
	// 10.3 second
	um = UnixUsec(time.Unix(10, 300_000_000))
	b, err = um.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, string("10.300000"), string(b))
	// 10.0003 second
	um = UnixUsec(time.Unix(10, 300_000))
	b, err = um.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, string("10.000300"), string(b))
}

func TestUnixUsecUnmarshalJSON(t *testing.T) {
	// 10.3 seconds
	var um UnixUsec
	err := um.UnmarshalJSON([]byte("10.3"))
	require.NoError(t, err)
	assert.Equal(t, UnixUsec(time.Unix(10, 300_000_000)), um)
	// 10.0003 seconds
	err = um.UnmarshalJSON([]byte("10.0003"))
	require.NoError(t, err)
	assert.Equal(t, UnixUsec(time.Unix(10, 300_000)), um)
	// 10.0000003, expect truncation of the digits below microsec
	err = um.UnmarshalJSON([]byte("10.0000003"))
	require.NoError(t, err)
	assert.Equal(t, UnixUsec(time.Unix(10, 0)), um)
}
