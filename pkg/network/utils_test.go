package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnakeToCamel(t *testing.T) {
	for test, exp := range map[string]string{
		"closed_conn_dropped":              "ClosedConnDropped",
		"closed_conn_polling_lost":         "ClosedConnPollingLost",
		"Conntrack_short_Term_Buffer_size": "ConntrackShortTermBufferSize",
	} {
		assert.Equal(t, exp, snakeToCapInitialCamel(test))
	}
}
