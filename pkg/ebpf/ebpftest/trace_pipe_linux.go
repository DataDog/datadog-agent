// Copyright 2017 Kinvolk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ebpftest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/ebpf-manager/tracefs"
)

// TracePipe to read from /sys/kernel/[debug/]tracing/trace_pipe
// Note that data can be read only once, i.e. if you have more than
// one tracer / channel, only one will receive an event:
// "Once data is read from this file, it is consumed, and will not be
// read again with a sequential read."
// https://www.kernel.org/doc/Documentation/trace/ftrace.txt
type TracePipe struct {
	file   *os.File
	reader *bufio.Reader
	stop   chan struct{}
}

// NewTracePipe instantiates a new trace pipe
func NewTracePipe() (*TracePipe, error) {
	f, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &TracePipe{
		file:   f,
		reader: bufio.NewReader(f),
		stop:   make(chan struct{}),
	}, nil
}

// A line from trace_pipe looks like (leading spaces included):
// `        chromium-15581 [000] d... 92783.722567: : Hello, World!`
var traceLineRegexp = regexp.MustCompile(`(.{16})-(\d+) +\[(\d{3})\] (.{4,5}) +(\d+\.\d+)\: (.*?)\: (.*)`)

func parseTraceLine(raw string) (*TraceEvent, error) {
	if raw == "\n" {
		return nil, nil
	}
	fields := traceLineRegexp.FindStringSubmatch(raw)
	if len(fields) != 8 {
		return nil, fmt.Errorf("received unexpected input %q", raw)
	}
	pid, _ := strconv.ParseUint(fields[2], 10, 32)

	return &TraceEvent{
		Raw:       raw,
		Task:      strings.Trim(fields[1], " "),
		PID:       uint32(pid),
		CPU:       fields[3],
		Flags:     fields[4],
		Timestamp: fields[5],
		Function:  fields[6],
		Message:   fields[7],
	}, nil
}

// ReadLine reads a line
func (t *TracePipe) ReadLine() (*TraceEvent, error) {
	line, err := t.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	traceEvent, err := parseTraceLine(line)
	if err != nil {
		return nil, err
	}
	return traceEvent, nil
}

// Channel returns a channel of events and another for errors
func (t *TracePipe) Channel() (<-chan *TraceEvent, <-chan error) {
	channelEvents := make(chan *TraceEvent)
	channelErrors := make(chan error)
	go func() {
		for {
			select {
			case <-t.stop:
				close(channelEvents)
				close(channelErrors)
				return
			default:
			}
			traceEvent, err := t.ReadLine()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, fs.ErrClosed) {
					continue
				}
				channelErrors <- err
			} else if traceEvent != nil {
				channelEvents <- traceEvent
			}
		}
	}()
	return channelEvents, channelErrors
}

// Close the trace pipe
func (t *TracePipe) Close() error {
	close(t.stop)
	return t.file.Close()
}
