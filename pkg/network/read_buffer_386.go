
// +build windows

package network

func AllocateReadBuffer() (*_readbuffer, error) {
	return nil, fmt.Errorf("unsupported on 386")
}

func FreeReadBuffer(buf * _readbuffer) {
	// this code is unreachable because AllocateReadBuffer will not succeed on 386
}