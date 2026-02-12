// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"fmt"
	"net"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// maxUDPDatagramSize is the maximum size of a UDP syslog datagram.
// RFC 5426 Section 3.1 recommends supporting at least 2048 bytes,
// and allows up to 65535 bytes for IPv4.
const maxUDPDatagramSize = 65535

// UDPTailer reads syslog messages from a UDP connection, parses them,
// and forwards structured messages to the pipeline.
//
// Each UDP datagram contains exactly one syslog message (RFC 5426),
// so no stream framing (Reader) is needed.
type UDPTailer struct {
	decoder     decoder.Decoder
	source      *sources.LogSource
	outputChan  chan *message.Message
	conn        *net.UDPConn
	stop        chan struct{}
	done        chan struct{}
	tagProvider tag.Provider
}

// NewUDPTailer returns a new syslog UDPTailer for the given UDP connection.
func NewUDPTailer(source *sources.LogSource, outputChan chan *message.Message, conn *net.UDPConn) *UDPTailer {
	return &UDPTailer{
		decoder:     decoder.NewNoopDecoder(),
		source:      source,
		outputChan:  outputChan,
		conn:        conn,
		stop:        make(chan struct{}, 1),
		done:        make(chan struct{}, 1),
		tagProvider: tag.NewLocalProvider(source.Config.Tags),
	}
}

// Start begins reading syslog datagrams from the UDP connection.
func (t *UDPTailer) Start() {
	t.source.Status.Success()
	log.Infof("Start tailing syslog UDP on %s", t.conn.LocalAddr())

	go t.forwardMessages()
	t.decoder.Start()
	go t.tail()
}

// Stop stops the tailer and waits for it to finish.
func (t *UDPTailer) Stop() {
	log.Infof("Stop tailing syslog UDP on %s", t.conn.LocalAddr())
	t.stop <- struct{}{}
	t.conn.Close()
	<-t.done
}

// Identifier returns a unique identifier for this tailer.
func (t *UDPTailer) Identifier() string {
	return fmt.Sprintf("syslog-udp:%s", t.conn.LocalAddr())
}

// forwardMessages reads decoded messages from the decoder output and
// forwards them to the pipeline output channel.
func (t *UDPTailer) forwardMessages() {
	defer func() {
		close(t.done)
	}()

	for decodedMessage := range t.decoder.OutputChan() {
		if len(decodedMessage.GetContent()) > 0 {
			t.outputChan <- decodedMessage
		}
	}
}

// tail reads syslog datagrams, parses them, and sends structured messages
// to the decoder's input channel.
func (t *UDPTailer) tail() {
	defer func() {
		t.decoder.Stop()
	}()

	buf := make([]byte, maxUDPDatagramSize)
	for {
		select {
		case <-t.stop:
			return
		default:
			n, addr, err := t.conn.ReadFromUDP(buf)
			if err != nil {
				if isClosedConn(err) {
					return
				}
				log.Warnf("Error reading syslog UDP datagram: %v", err)
				return
			}
			if n == 0 {
				continue
			}

			t.source.RecordBytes(int64(n))

			// Make a copy of the datagram data; buf is reused.
			frame := make([]byte, n)
			copy(frame, buf[:n])

			origin := t.getOrigin()
			if addr != nil && pkgconfigsetup.Datadog().GetBool("logs_config.use_sourcehost_tag") {
				tags := origin.Tags(nil)
				origin.SetTags(append(tags, "source_host:"+addr.IP.String()))
			}

			msg, err := buildStructuredMessage(frame, origin)
			if err != nil {
				log.Debugf("Error parsing syslog UDP message from %s: %v", addr, err)
			}

			select {
			case <-t.stop:
				return
			case t.decoder.InputChan() <- msg:
			}
		}
	}
}

// getOrigin returns a new message origin for this tailer.
func (t *UDPTailer) getOrigin() *message.Origin {
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.SetTags(t.tagProvider.GetTags())
	return origin
}

// isClosedConn returns true if the error is related to a closed connection.
func isClosedConn(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
