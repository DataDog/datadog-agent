// +build linux

package afpacket

// Fd returns the file descriptor of the underlying socket.
func (h *TPacket) Fd() int {
	return h.fd
}
