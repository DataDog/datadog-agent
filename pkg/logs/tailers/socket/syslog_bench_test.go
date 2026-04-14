// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// ---------------------------------------------------------------------------
// Test data: representative syslog messages at various sizes
// ---------------------------------------------------------------------------

var (
	rfc5424Short   = []byte(`<14>1 2003-10-11T22:14:15.003Z host app - - - short`)
	rfc5424Typical = []byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry`)
	rfc5424Long    = []byte(`<14>1 2003-10-11T22:14:15.003Z longhost.example.com myservice 12345 REQ-001 [meta@1234 key="val"] ` + strings.Repeat("x", 1024))
	bsdTypical     = []byte(`<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`)
)

// ---------------------------------------------------------------------------
// Parse (syslog field extraction)
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
// Render
// ---------------------------------------------------------------------------

func BenchmarkRender(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		parsed, _ := syslogparser.Parse(tc.msg)
		sc := &message.BasicStructuredContent{
			Data: map[string]interface{}{
				"message": string(parsed.Msg),
				"syslog":  syslogparser.BuildSyslogFields(parsed),
			},
		}
		source := sources.NewLogSource("bench", &config.LogsConfig{})
		origin := message.NewOrigin(source)
		msg := message.NewStructuredMessage(sc, origin, syslogparser.SeverityToStatus(parsed.Pri), time.Now().UnixNano())
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
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
// Render (SyslogStructuredContent via jsoniter.Stream)
// ---------------------------------------------------------------------------

func BenchmarkRenderNew(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		parsed, _ := syslogparser.Parse(tc.msg)
		sc := syslogparser.NewSyslogStructuredContent(parsed)
		source := sources.NewLogSource("bench", &config.LogsConfig{})
		origin := message.NewOrigin(source)
		msg := message.NewStructuredMessage(sc, origin, syslogparser.SeverityToStatus(parsed.Pri), time.Now().UnixNano())
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
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
// Render + EncodeFull: the real new path (render once, wrap once)
// ---------------------------------------------------------------------------

func BenchmarkRenderAndEncodeFull(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		parsed, _ := syslogparser.Parse(tc.msg)
		sc := syslogparser.NewSyslogStructuredContent(parsed)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rendered, err := sc.Render()
				if err != nil {
					b.Fatal(err)
				}
				_, err = sc.EncodeFull(rendered, "info", 1699000000000,
					"myhost", "myservice", "mysource", "env:prod,team:logs")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRenderThenMarshal measures the real old double-serialization path:
// Render() produces inner JSON, then encoding/json.Marshal wraps it using
// processor.ValidUtf8Bytes (a TextMarshaler) which encodes the rendered JSON
// as a JSON string value, re-escaping all quotes.
func BenchmarkRenderThenMarshal(b *testing.B) {
	type transportPayload struct {
		Message   processor.ValidUtf8Bytes `json:"message"`
		Status    string                   `json:"status"`
		Timestamp int64                    `json:"timestamp"`
		Hostname  string                   `json:"hostname"`
		Service   string                   `json:"service"`
		Source    string                   `json:"ddsource"`
		Tags      string                   `json:"ddtags"`
	}

	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		parsed, _ := syslogparser.Parse(tc.msg)
		sc := syslogparser.NewSyslogStructuredContent(parsed)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rendered, err := sc.Render()
				if err != nil {
					b.Fatal(err)
				}
				_, err = json.Marshal(transportPayload{
					Message:   processor.ValidUtf8Bytes(rendered),
					Status:    "info",
					Timestamp: 1699000000000,
					Hostname:  "myhost",
					Service:   "myservice",
					Source:    "mysource",
					Tags:      "env:prod,team:logs",
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEncodeViaProcessor measures the actual Encode() path through the
// processor, which now uses the FullEncoder fast path for syslog messages.
func BenchmarkEncodeViaProcessor(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Short", rfc5424Short},
		{"RFC5424_Typical", rfc5424Typical},
		{"RFC5424_Long_1KB", rfc5424Long},
		{"BSD", bsdTypical},
	} {
		parsed, _ := syslogparser.Parse(tc.msg)
		sc := syslogparser.NewSyslogStructuredContent(parsed)
		source := sources.NewLogSource("bench", &config.LogsConfig{})
		origin := message.NewOrigin(source)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				msg := message.NewStructuredMessage(sc, origin, syslogparser.SeverityToStatus(parsed.Pri), time.Now().UnixNano())
				rendered, err := msg.Render()
				if err != nil {
					b.Fatal(err)
				}
				msg.SetRendered(rendered)
				err = processor.JSONEncoder.Encode(msg, "myhost")
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End-to-end with net.Pipe through StreamTailer (syslog format)
// ---------------------------------------------------------------------------

func BenchmarkStreamTailerSyslogEndToEnd(b *testing.B) {
	for _, tc := range []struct {
		name string
		msg  []byte
	}{
		{"RFC5424_Typical", rfc5424Typical},
	} {
		b.Run(tc.name, func(b *testing.B) {
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()

			source := sources.NewLogSource("bench-syslog", &config.LogsConfig{Format: config.SyslogFormat})
			outputChan := make(chan *message.Message, 256)

			tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, 4096, 0, "")
			tailer.Start()

			line := append(tc.msg, '\n')
			b.SetBytes(int64(len(line)))
			b.ResetTimer()

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
