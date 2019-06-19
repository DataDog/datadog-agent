package ebpf

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strconv"
	"syscall"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	psnet "github.com/DataDog/gopsutil/net"
)

const tcpListen = 10

// ActiveTCPConns returns a set of active tcp connections where the keys
// are encoded connections keys
func ActiveTCPConns(buf *bytes.Buffer) (map[string]struct{}, error) {
	conns, err := psnet.Connections("tcp")
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving active tcp connections")
	}

	keys := map[string]struct{}{}
	for _, c := range conns {
		cs, ok := formatPsutilConn(c)
		if !ok {
			continue
		}

		bk, err := cs.ByteKey(buf)
		if err != nil {
			log.Warnf("error building connection byte key: %s", err)
			continue
		}

		keys[string(bk)] = struct{}{}
	}

	return keys, nil
}

// formatPsutilConn formats a gopsutil connection into a ConnectionStats struct
// It also returns a boolean indicating if we should or not process the connection
func formatPsutilConn(c psnet.ConnectionStat) (ConnectionStats, bool) {
	cs := ConnectionStats{
		Pid:    uint32(c.Pid),
		Source: util.AddressFromString(c.Laddr.IP),
		Dest:   util.AddressFromString(c.Raddr.IP),
		SPort:  uint16(c.Laddr.Port),
		DPort:  uint16(c.Raddr.Port),
	}

	if cs.Pid == 0 || cs.SPort == 0 || cs.DPort == 0 {
		return cs, false
	}

	if c.Family == syscall.AF_INET {
		cs.Family = AFINET
	} else if c.Family == syscall.AF_INET6 {
		// Check if the IPv6 is mapped to IPv4
		isV4 := net.ParseIP(c.Laddr.IP).To4() != nil || net.ParseIP(c.Raddr.IP).To4() != nil
		if isV4 {
			cs.Family = AFINET
		} else {
			cs.Family = AFINET6
		}
	} else {
		return cs, false
	}

	if c.Type == syscall.SOCK_STREAM {
		cs.Type = TCP
	} else if c.Type == syscall.SOCK_DGRAM {
		cs.Type = UDP
	} else {
		return cs, false
	}

	return cs, true
}

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
