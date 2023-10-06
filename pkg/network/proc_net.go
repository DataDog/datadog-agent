// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tcpListen int64 = 10

	// tcpClose is also used to indicate a UDP connection where the other end hasn't been established
	tcpClose int64 = 7
)

// readProcNetListeners reads a /proc/net/ file and returns a list of all source ports for connections in the tcpListen state
func readProcNetListeners(path string) ([]uint16, error) {
	return readProcNetWithStatus(path, tcpListen)
}

// readProcNet reads a /proc/net/ file and returns a list of all source ports for connections in the given state
func readProcNetWithStatus(path string, status int64) ([]uint16, error) {
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

			if state != status {
				continue
			}

			idx := bytes.IndexByte(rawLocal, ':')
			if idx == -1 {
				continue
			}

			port, err := strconv.ParseUint(string(rawLocal[idx+1:]), 16, 16)
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
