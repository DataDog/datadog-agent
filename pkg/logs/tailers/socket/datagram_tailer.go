// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"fmt"
	"net"
	"strings"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// maxDatagramSize is the maximum size of a UDP/unixgram datagram.
// RFC 5426 Section 3.1 recommends supporting at least 2048 bytes,
// and allows up to 65535 bytes for IPv4.
const maxDatagramSize = 65535

// DatagramTailer reads log messages from a datagram-oriented connection
// (UDP or Unix datagram). Each datagram is one complete message.
//
// The source's Format field controls whether syslog or unstructured
// parsing is used — the decoder handles all format-specific logic.
//
// source_host tagging is only applied for UDP connections (not unixgram).
type DatagramTailer struct {
	source     *sources.LogSource
	conn       net.PacketConn
	outputChan chan *message.Message
	isIPBased  bool
	frameSize  int
	decoder    decoder.Decoder
	stop       chan struct{}
	done       chan struct{}
	onError    func() // called when tail() exits due to a transient read error; used by listeners to reset the connection
}

// NewDatagramTailer returns a new DatagramTailer.
// conn must be a net.PacketConn (e.g. *net.UDPConn or *net.UnixConn).
// Parsing is controlled by source.Config.Format (config.SyslogFormat for
// syslog, "" for unstructured).
// isUDP controls whether source_host tagging is applied.
// frameSize limits the read buffer for unstructured mode; use 0 for no limit.
func NewDatagramTailer(source *sources.LogSource, conn net.PacketConn, outputChan chan *message.Message, isIPBased bool, frameSize int) *DatagramTailer {
	replSource := sources.NewReplaceableSource(source)
	tailerInfo := status.NewInfoRegistry()

	return &DatagramTailer{
		source:     source,
		conn:       conn,
		outputChan: outputChan,
		isIPBased:  isIPBased,
		frameSize:  frameSize,
		decoder:    decoder.NewDatagramDecoder(replSource, tailerInfo),
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Start begins reading datagrams from the connection.
func (t *DatagramTailer) Start() {
	t.source.Status.Success()
	log.Infof("Start tailing datagrams on %s (format=%q, udp=%v)", t.conn.LocalAddr(), t.source.Config.Format, t.isIPBased)

	go t.forwardMessages()
	t.decoder.Start()
	go t.tail()
}

// Stop stops the tailer and waits for it to finish.
func (t *DatagramTailer) Stop() {
	log.Infof("Stop tailing datagrams on %s", t.conn.LocalAddr())
	t.stop <- struct{}{}
	t.conn.Close()
	<-t.done
}

// Identifier returns a unique identifier for this tailer.
func (t *DatagramTailer) Identifier() string {
	return fmt.Sprintf("datagram:%s", t.conn.LocalAddr())
}

// SetOnError registers a callback invoked when the read loop exits
// due to a transient error (not an explicit stop or closed connection).
// Must be called before Start.
func (t *DatagramTailer) SetOnError(fn func()) {
	t.onError = fn
}

// forwardMessages reads decoded messages from the decoder and forwards
// them to the pipeline output channel with a properly configured origin.
// For syslog format, it applies source/service overrides from ParsingExtra
// (set by the syslog parser).
func (t *DatagramTailer) forwardMessages() {
	defer func() {
		close(t.done)
	}()

	for output := range t.decoder.OutputChan() {
		if len(output.GetContent()) > 0 {
			origin := message.NewOrigin(t.source)
			origin.Identifier = t.Identifier()
			origin.SetTags(output.ParsingExtra.Tags)

			if output.ParsingExtra.SourceOverride != "" {
				origin.SetSource(output.ParsingExtra.SourceOverride)
			}
			if output.ParsingExtra.ServiceOverride != "" {
				origin.SetService(output.ParsingExtra.ServiceOverride)
			}

			output.Origin = origin
			t.outputChan <- output
		}
	}
}

// tail reads datagrams and feeds them to the decoder.
func (t *DatagramTailer) tail() {
	defer func() {
		t.decoder.Stop()
	}()

	buf := make([]byte, maxDatagramSize)
	for {
		select {
		case <-t.stop:
			return
		default:
			n, addr, err := t.conn.ReadFrom(buf)
			if err != nil {
				if isClosedConn(err) {
					return
				}
				log.Warnf("Error reading datagram: %v", err)
				if t.onError != nil {
					go t.onError()
				}
				return
			}
			if n == 0 {
				continue
			}

			// Truncate to frameSize if configured, matching legacy UDP behavior
			readLen := n
			if t.frameSize > 0 && readLen > t.frameSize {
				readLen = t.frameSize
			}

			data := make([]byte, readLen)
			copy(data, buf[:readLen])

			// For unstructured mode (UTF-8 newline framing), ensure the
			// datagram ends with a newline so the framer emits it promptly.
			// Syslog mode uses NoFraming where this is unnecessary.
			if t.source.Config.Format != logsconfig.SyslogFormat && len(data) > 0 && data[len(data)-1] != '\n' {
				data = append(data, '\n')
			}

			t.source.RecordBytes(int64(n))

			msg := decoder.NewInput(data)
			if t.isIPBased && addr != nil && pkgconfigsetup.Datadog().GetBool("logs_config.use_sourcehost_tag") {
				ipStr := extractIP(addr)
				if ipStr != "" {
					msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, "source_host:"+ipStr)
				}
			}

			t.decoder.InputChan() <- msg
		}
	}
}

// extractIP extracts the IP address string from a net.Addr.
func extractIP(addr net.Addr) string {
	addrStr := addr.String()
	lastColon := strings.LastIndex(addrStr, ":")
	if lastColon != -1 {
		return addrStr[:lastColon]
	}
	return addrStr
}

// isClosedConn returns true if the error is related to a closed connection.
func isClosedConn(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
