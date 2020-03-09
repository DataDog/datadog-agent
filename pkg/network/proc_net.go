package network

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const tcpListen = 10

// readProcNet reads a /proc/net/ file and returns a list of all ports being listened on
func readProcNet(path string) ([]uint16, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	reader := bufio.NewReader(f)

	ports := make([]uint16, 0)

	// Skip header line
	_, _ = reader.ReadBytes('\n')

	for {
		var rawLocal, rawState []byte

		b, err := reader.ReadBytes('\n')

		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else {
			iter := &fieldIterator{data: b}
			iter.nextField() // entry number

			rawLocal = iter.nextField() // local_address

			iter.nextField() // remote_address

			rawState = iter.nextField() // st

			state, err := strconv.ParseInt(string(rawState), 16, 0)
			if err != nil {
				log.Errorf("error parsing tcp state [%s] as hex: %s", rawState, err)
				continue
			}

			if state != tcpListen {
				continue
			}

			idx := bytes.IndexByte(rawLocal, ':')
			if idx == -1 {
				continue
			}

			port, err := strconv.ParseInt(string(rawLocal[idx+1:]), 16, 0)
			if err != nil {
				log.Errorf("error parsing port [%s] as hex: %s", rawLocal[idx+1:], err)
				continue
			}

			ports = append(ports, uint16(port))

		}
	}

	return ports, nil
}

type fieldIterator struct {
	data []byte
}

func (iter *fieldIterator) nextField() []byte {
	// Skip any leading whitespace
	for i, b := range iter.data {
		if b != ' ' {
			iter.data = iter.data[i:]
			break
		}
	}

	// Read field up until the first whitespace char
	var result []byte
	for i, b := range iter.data {
		if b == ' ' {
			result = iter.data[:i]
			iter.data = iter.data[i:]
			break
		}
	}

	return result
}
