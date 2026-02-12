// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// benchBuildStructuredMessage wraps the unexported buildStructuredMessage
// for benchmark access. We use this instead of the syslogparser import to
// exercise the actual production code path.
func benchBuildStructuredMessage(frame []byte, origin *message.Origin) (*message.Message, error) {
	return buildStructuredMessage(frame, origin)
}

// ---------------------------------------------------------------------------
// Test data: representative syslog messages at various sizes
// ---------------------------------------------------------------------------

var (
	// Short RFC 5424 (~90 bytes)
	rfc5424Short = []byte(`<14>1 2003-10-11T22:14:15.003Z host app - - - short`)

	// Typical RFC 5424 with structured data (~180 bytes)
	rfc5424Typical = []byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry`)

	// Long RFC 5424 with ~1KB message body
	rfc5424Long = []byte(`<14>1 2003-10-11T22:14:15.003Z longhost.example.com myservice 12345 REQ-001 [meta@1234 key="val"] ` + strings.Repeat("x", 1024))

	// BSD format (~80 bytes)
	bsdTypical = []byte(`<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`)
)

// ---------------------------------------------------------------------------
// Layer 1: Reader.ReadFrame (framing only)
// ---------------------------------------------------------------------------

// cycleReader is an io.Reader that repeats data endlessly. This avoids
// allocating a massive buffer proportional to b.N and prevents OOM for
// large messages.
type cycleReader struct {
	data []byte
	pos  int
}

func (cr *cycleReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		copied := copy(p[n:], cr.data[cr.pos:])
		n += copied
		cr.pos += copied
		if cr.pos >= len(cr.data) {
			cr.pos = 0
		}
	}
	return n, nil
}

func BenchmarkReader_NonTransparent(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"Short", rfc5424Short},
		{"Typical", rfc5424Typical},
		{"Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			line := append(append([]byte{}, tc.msg...), '\n')
			reader := NewReader(&cycleReader{data: line})
			b.SetBytes(int64(len(line)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := reader.ReadFrame()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkReader_OctetCounted(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"Short", rfc5424Short},
		{"Typical", rfc5424Typical},
		{"Long_1KB", rfc5424Long},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			frame := []byte(fmt.Sprintf("%d %s", len(tc.msg), tc.msg))
			reader := NewReader(&cycleReader{data: frame})
			b.SetBytes(int64(len(frame)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := reader.ReadFrame()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 2: Parse (syslog field extraction)
// ---------------------------------------------------------------------------

func BenchmarkParse(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.msg)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = syslogparser.Parse(tc.msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 3: BuildStructuredMessage (parse + structured content + message)
// ---------------------------------------------------------------------------

func BenchmarkBuildStructuredMessage(b *testing.B) {
	source := sources.NewLogSource("bench", &config.LogsConfig{})

	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.msg)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				origin := message.NewOrigin(source)
				_, _ = benchBuildStructuredMessage(tc.msg, origin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 4: Render (JSON serialization of structured content)
// ---------------------------------------------------------------------------

func BenchmarkRender(b *testing.B) {
	source := sources.NewLogSource("bench", &config.LogsConfig{})

	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		origin := message.NewOrigin(source)
		msg, _ := benchBuildStructuredMessage(tc.msg, origin)
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := msg.Render()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 5: Frame copy (the defensive copy in the tailer)
// ---------------------------------------------------------------------------

func BenchmarkFrameCopy(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"Short", rfc5424Short},
		{"Typical", rfc5424Typical},
		{"Long_1KB", rfc5424Long},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.msg)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				owned := make([]byte, len(tc.msg))
				copy(owned, tc.msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End-to-end: full tailer pipeline (ReadFrame + copy + BuildStructuredMessage)
// ---------------------------------------------------------------------------

func BenchmarkTailerPipeline(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			line := append(append([]byte{}, tc.msg...), '\n')

			source := sources.NewLogSource("bench", &config.LogsConfig{})
			reader := NewReader(&cycleReader{data: line})

			b.SetBytes(int64(len(line)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				frame, err := reader.ReadFrame()
				if err != nil {
					b.Fatal(err)
				}

				origin := message.NewOrigin(source)
				msg, _ := benchBuildStructuredMessage(frame, origin)

				// Simulate what the pipeline does: render
				_, err = msg.Render()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End-to-end with net.Pipe: full tailer including goroutines and channels
// ---------------------------------------------------------------------------

func BenchmarkTailerEndToEnd(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Typical", rfc5424Typical},
	} {
		b.Run(tc.name, func(b *testing.B) {
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()

			source := sources.NewLogSource("bench-syslog", &config.LogsConfig{})
			outputChan := make(chan *message.Message, 256)

			tailer := NewTailer(source, outputChan, serverConn)
			tailer.Start()

			line := append(tc.msg, '\n')
			b.SetBytes(int64(len(line)))
			b.ResetTimer()

			// Writer goroutine: feed all messages
			done := make(chan struct{})
			go func() {
				defer close(done)
				for i := 0; i < b.N; i++ {
					_, err := clientConn.Write(line)
					if err != nil {
						return
					}
				}
				clientConn.Close()
			}()

			// Consumer: drain output channel
			received := 0
			timeout := time.After(30 * time.Second)
			for received < b.N {
				select {
				case <-outputChan:
					received++
				case <-timeout:
					b.Fatalf("timeout after receiving %d/%d messages", received, b.N)
				}
			}
			b.StopTimer()

			<-done
			tailer.Stop()
		})
	}
}

// ---------------------------------------------------------------------------
// Allocation-focused: count allocations per message
// ---------------------------------------------------------------------------

func BenchmarkAllocations_BuildStructuredMessage(b *testing.B) {
	source := sources.NewLogSource("bench", &config.LogsConfig{})

	b.Run("RFC5424_Typical", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			origin := message.NewOrigin(source)
			msg, _ := benchBuildStructuredMessage(rfc5424Typical, origin)
			// Force render to exercise the full allocation chain
			msg.Render() //nolint:errcheck
		}
	})
}

func BenchmarkAllocations_FrameCopyVsNoCopy(b *testing.B) {
	source := sources.NewLogSource("bench", &config.LogsConfig{})

	b.Run("WithCopy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			owned := make([]byte, len(rfc5424Typical))
			copy(owned, rfc5424Typical)
			origin := message.NewOrigin(source)
			benchBuildStructuredMessage(owned, origin) //nolint:errcheck
		}
	})

	b.Run("NoCopy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			origin := message.NewOrigin(source)
			benchBuildStructuredMessage(rfc5424Typical, origin) //nolint:errcheck
		}
	})
}

// sink prevents compiler optimization from eliminating benchmark work
var sink interface{}

func init() {
	_ = io.Discard // ensure io is used
	_ = sink
}
