// +build !windows

package ipc

import "net"

const sockPath = "/tmp/agent.sock"

// getListener returns a listening connection to a Unix socket
// on non-windows platforms.
func getListener() (net.Listener, error) {
	return net.Listen("unix", sockPath)
}

// GetConn returns a dialling connection to a Unix socket
// on non-windows platforms. This method is exported so it
// can be used by clients.
func GetConn() (net.Conn, error) {
	return net.Dial("unix", sockPath)
}
