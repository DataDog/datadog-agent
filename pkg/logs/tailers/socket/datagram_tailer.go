// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"fmt"
	"net"
	"strings"

	config "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
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
// When format is "syslog", each datagram is parsed as a syslog message
// and forwarded as StateStructured. Otherwise, datagrams are forwarded
// as unstructured lines through the decoder.
//
// source_host tagging is only applied for UDP connections (not unixgram).
type DatagramTailer struct {
	source      *sources.LogSource
	conn        net.PacketConn
	outputChan  chan *message.Message
	format      string
	isUDP       bool
	frameSize   int
	stop        chan struct{}
	done        chan struct{}
	tagProvider tag.Provider
	// Used only for unstructured mode
	decoder decoder.Decoder
	// Used only for syslog mode
	noopDecoder decoder.Decoder
}

// NewDatagramTailer returns a new DatagramTailer.
// conn must be a net.PacketConn (e.g. *net.UDPConn or *net.UnixConn).
// format determines parsing: config.SyslogFormat for syslog, "" for unstructured.
// isUDP controls whether source_host tagging is applied.
// frameSize limits the read buffer for unstructured mode; use 0 for no limit.
func NewDatagramTailer(source *sources.LogSource, conn net.PacketConn, outputChan chan *message.Message, format string, isUDP bool, frameSize int) *DatagramTailer {
	t := &DatagramTailer{
		source:      source,
		conn:        conn,
		outputChan:  outputChan,
		format:      format,
		isUDP:       isUDP,
		frameSize:   frameSize,
		stop:        make(chan struct{}, 1),
		done:        make(chan struct{}, 1),
		tagProvider: tag.NewLocalProvider(source.Config.Tags),
	}
	if format == config.SyslogFormat {
		t.noopDecoder = decoder.NewNoopDecoder()
	} else {
		t.decoder = decoder.InitializeDecoder(sources.NewReplaceableSource(source), noop.New(), status.NewInfoRegistry())
	}
	return t
}

// Start begins reading datagrams from the connection.
func (t *DatagramTailer) Start() {
	t.source.Status.Success()
	log.Infof("Start tailing datagrams on %s (format=%q, udp=%v)", t.conn.LocalAddr(), t.format, t.isUDP)

	go t.forwardMessages()
	if t.format == config.SyslogFormat {
		t.noopDecoder.Start()
	} else {
		t.decoder.Start()
	}
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

// forwardMessages reads decoded messages from the active decoder and
// forwards them to the pipeline output channel.
func (t *DatagramTailer) forwardMessages() {
	defer func() {
		close(t.done)
	}()

	var outChan <-chan *message.Message
	if t.format == config.SyslogFormat {
		outChan = t.noopDecoder.OutputChan()
	} else {
		outChan = t.decoder.OutputChan()
	}

	for decodedMessage := range outChan {
		if len(decodedMessage.GetContent()) > 0 {
			if t.format != config.SyslogFormat {
				origin := message.NewOrigin(t.source)
				origin.SetTags(decodedMessage.ParsingExtra.Tags)
				msg := message.NewMessageWithParsingExtra(decodedMessage.GetContent(), origin, decodedMessage.Status, decodedMessage.IngestionTimestamp, decodedMessage.ParsingExtra)
				t.outputChan <- msg
			} else {
				t.outputChan <- decodedMessage
			}
		}
	}
}

// tail reads datagrams and dispatches them based on format.
func (t *DatagramTailer) tail() {
	defer func() {
		if t.format == config.SyslogFormat {
			t.noopDecoder.Stop()
		} else {
			t.decoder.Stop()
		}
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
				return
			}
			if n == 0 {
				continue
			}

			t.source.RecordBytes(int64(n))

			if t.format == config.SyslogFormat {
				t.handleSyslogDatagram(buf[:n], addr)
			} else {
				t.handleUnstructuredDatagram(buf[:n], addr)
			}
		}
	}
}

// handleSyslogDatagram parses a syslog datagram and sends a structured message.
func (t *DatagramTailer) handleSyslogDatagram(data []byte, addr net.Addr) {
	frame := make([]byte, len(data))
	copy(frame, data)

	origin := t.getOrigin()
	if t.isUDP && addr != nil && pkgconfigsetup.Datadog().GetBool("logs_config.use_sourcehost_tag") {
		tags := origin.Tags(nil)
		ipStr := extractIP(addr)
		if ipStr != "" {
			origin.SetTags(append(tags, "source_host:"+ipStr))
		}
	}

	msg, err := buildSyslogStructuredMessage(frame, origin)
	if err != nil {
		log.Debugf("Error parsing syslog datagram from %s: %v", addr, err)
	}

	select {
	case <-t.stop:
		return
	case t.noopDecoder.InputChan() <- msg:
	}
}

// handleUnstructuredDatagram sends raw datagram data through the decoder pipeline.
func (t *DatagramTailer) handleUnstructuredDatagram(data []byte, addr net.Addr) {
	n := len(data)

	// Truncate to frameSize if configured, matching legacy UDP behavior
	if t.frameSize > 0 && n > t.frameSize {
		n = t.frameSize
	}

	frame := make([]byte, n, n+1)
	copy(frame, data[:n])

	// Ensure lines end with newline for proper splitting downstream
	if len(frame) > 0 && frame[len(frame)-1] != '\n' {
		frame = append(frame, '\n')
	}

	input := decoder.NewInput(frame)

	if t.isUDP && addr != nil && pkgconfigsetup.Datadog().GetBool("logs_config.use_sourcehost_tag") {
		ipStr := extractIP(addr)
		if ipStr != "" {
			input.ParsingExtra.Tags = append(input.ParsingExtra.Tags, "source_host:"+ipStr)
		}
	}

	t.decoder.InputChan() <- input
}

// getOrigin returns a new message origin for this tailer.
func (t *DatagramTailer) getOrigin() *message.Origin {
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.SetTags(t.tagProvider.GetTags())
	return origin
}

// extractIP extracts the IP address string from a net.Addr.
func extractIP(addr net.Addr) string {
	addrStr := addr.String()
	// For UDP addresses, strip the port
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
