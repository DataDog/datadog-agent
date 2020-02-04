// +build linux

package net

import (
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func testSocketExistsNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a socket
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	assert.NoError(t, err)
	_, err = net.Listen("unix", addr.Name)
	assert.NoError(t, err)

	// Create a new socket using UDSListener
	l, err := NewUDSListener(&config.AgentConfig{SystemProbeSocketPath: socketPath})
	require.NoError(t, err)

	l.Stop()
}

func testSocketExistsAsRegularFileNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a file
	f, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	assert.NoError(t, err)
	defer f.Close()

	// Create a new socket using UDSListener
	_, err = NewUDSListener(&config.AgentConfig{SystemProbeSocketPath: socketPath})
	require.Error(t, err)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewUDSListener(&config.AgentConfig{SystemProbeSocketPath: socketPath})
	require.NoError(t, err)
	defer s.Stop()

	assert.NoError(t, err)
	assert.NotNil(t, s)
	time.Sleep(1 * time.Second)
	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	t.Run("socket_exists_but_is_successfully_removed", func(tt *testing.T) {
		dir, _ := ioutil.TempDir("", "dd-test-")
		defer os.RemoveAll(dir) // clean up after
		testSocketExistsNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("non_socket_exists_and_fails_to_be_removed", func(tt *testing.T) {
		dir, _ := ioutil.TempDir("", "dd-test-")
		defer os.RemoveAll(dir) // clean up after
		testSocketExistsAsRegularFileNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("working", func(tt *testing.T) {
		dir, _ := ioutil.TempDir("", "dd-test-")
		defer os.RemoveAll(dir) // clean up after
		testWorkingNewUDSListener(tt, dir+"/net.sock")
	})
}
