// +build !linux

package listeners

import {
    "fmt"
    "net"
}

const oob_size = 0

// enablePassCred returns a "not implemented" error on non-linux hosts
func enablePassCred(conn *net.UnixConn) error {
	return fmt.Errorf("only implemented on Linux hosts")
}

func processOrigin(oob []byte) (string, error) {
    return "", fmt.Errorf("only implemented on Linux hosts")
}
