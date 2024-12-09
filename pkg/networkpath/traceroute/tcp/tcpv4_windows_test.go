package tcp

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestReserveTCPPort(t *testing.T) {
	// WHEN we reserve a local port
	port, socket, err := reserveTCPPort()
	require.NoError(t, err)
	defer windows.Closesocket(socket)
	require.NotEqual(t, socket, windows.InvalidHandle)
	assert.NotEqual(t, 0, port)

	// THEN we should not be able to get another connection
	// on the same port
	conn2, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	assert.Error(t, err)
	assert.Equal(t, "something", err.Error())
	assert.Nil(t, conn2)
}
