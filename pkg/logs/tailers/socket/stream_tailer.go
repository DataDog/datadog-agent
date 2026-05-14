// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package socket provides socket-based log tailers
package socket

import (
	"io"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StreamTailer reads data from a stream-oriented connection (TCP or Unix
// stream) and sends log messages to the pipeline.
//
// The source's Format field controls how the byte stream is framed and parsed:
//   - "" (default): UTF-8 newline framing with a noop parser (unstructured)
//   - "syslog": RFC 6587 syslog framing (octet counting / non-transparent)
//     with a syslog parser producing StateStructured messages
type StreamTailer struct {
	source          *sources.LogSource
	Conn            net.Conn
	outputChan      chan *message.Message
	frameSize       int
	idleTimeout     time.Duration
	sourceHostAddr  string
	decoder         decoder.Decoder
	capacityMonitor *metrics.CapacityMonitor
	stop            chan struct{}
	done            chan struct{}
	onDone          func() // called when readLoop exits (connection closed/error); used by listeners to prune dead tailers
}

// NewStreamTailer returns a new StreamTailer.
//
// Parameters:
//   - source: the log source configuration (Format controls framing/parsing)
//   - conn: the stream connection to read from
//   - outputChan: channel for forwarding parsed messages to the pipeline
//   - frameSize: buffer size for conn.Read()
//   - idleTimeout: idle read timeout (0 means no timeout)
//   - sourceHostAddr: pre-extracted remote IP for source_host tagging ("" to skip)
func NewStreamTailer(source *sources.LogSource, conn net.Conn, outputChan chan *message.Message, frameSize int, idleTimeout time.Duration, sourceHostAddr string, capacityMonitor *metrics.CapacityMonitor) *StreamTailer {
	replSource := sources.NewReplaceableSource(source)
	tailerInfo := status.NewInfoRegistry()

	dec := decoder.NewStreamDecoder(replSource, tailerInfo)

	return &StreamTailer{
		source:          source,
		Conn:            conn,
		outputChan:      outputChan,
		frameSize:       frameSize,
		idleTimeout:     idleTimeout,
		sourceHostAddr:  sourceHostAddr,
		decoder:         dec,
		capacityMonitor: capacityMonitor,
		stop:            make(chan struct{}, 1),
		done:            make(chan struct{}, 1),
	}
}

// Start begins reading and decoding data from the connection.
func (t *StreamTailer) Start() {
	t.source.Status.Success()
	log.Infof("Start tailing stream from %s (format=%q)", t.Conn.RemoteAddr(), t.source.Config.Format)

	go t.forwardMessages()
	t.decoder.Start()
	go t.readLoop()
}

// Stop stops the tailer and waits for the decoder to be flushed.
func (t *StreamTailer) Stop() {
	log.Infof("Stop tailing stream from %s", t.Conn.RemoteAddr())
	t.stop <- struct{}{}
	t.Conn.Close()
	<-t.done
}

// SetOnDone registers a callback invoked when the readLoop exits
// (connection closed, error, or EOF). Must be called before Start.
func (t *StreamTailer) SetOnDone(fn func()) {
	t.onDone = fn
}

// forwardMessages reads decoded messages from the decoder output and
// forwards them to the pipeline output channel with a properly configured
// origin.
func (t *StreamTailer) forwardMessages() {
	defer func() {
		close(t.done)
	}()

	for output := range t.decoder.OutputChan() {
		if output.HasContent() {
			origin := message.NewOrigin(t.source)
			origin.SetTags(output.ParsingExtra.Tags)

			output.Origin = origin
			t.outputChan <- output
			t.capacityMonitor.AddIngress(output)
		}
	}
}

// readLoop reads raw bytes from the connection and feeds them to the
// decoder. Idle timeout and source_host tagging are handled here.
func (t *StreamTailer) readLoop() {
	defer func() {
		if t.onDone != nil {
			go t.onDone()
		}
		t.Conn.Close()
		t.decoder.Stop()
	}()

	buf := make([]byte, t.frameSize)
	for {
		select {
		case <-t.stop:
			return
		default:
			if t.idleTimeout > 0 {
				t.Conn.SetReadDeadline(time.Now().Add(t.idleTimeout)) //nolint:errcheck
			}

			n, err := t.Conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Warnf("Couldn't read message from connection %s: %v", t.Conn.RemoteAddr(), err)
				return
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			t.source.RecordBytes(int64(n))
			msg := decoder.NewInput(data)

			if t.sourceHostAddr != "" && pkgconfigsetup.Datadog().GetBool("logs_config.use_sourcehost_tag") {
				msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, "source_host:"+t.sourceHostAddr)
			}

			t.decoder.InputChan() <- msg
		}
	}
}
