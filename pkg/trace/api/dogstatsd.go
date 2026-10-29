// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bufio"
	"net"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// maxDogstatsdProxyLines bounds the number of lines read from the body of a
// single proxied request. Without it, both the number of UDP writes and the
// number of scanner iterations one request causes are only bounded by the
// request size, so a body of two-byte lines can drive one write per two bytes
// sent, and a body of newlines one iteration per byte sent. Empty lines relay
// nothing but still count, so that the work a request can ask for stays a fixed
// cost. The limit stays well above any realistic batch, which is a handful of
// metrics per request.
const maxDogstatsdProxyLines = 100_000

// maxDogstatsdProxyPacketSize caps packed datagrams. It matches the
// dogstatsd_buffer_size default and fits every platform's UDP limit (macOS:
// 9216). Larger packets would save few writes.
const maxDogstatsdProxyPacketSize = 8192

// dogstatsdProxyPacketSize returns the size to pack lines up to, or 0 to send
// one line per datagram. It never exceeds bufferSize, as DogStatsD truncates
// larger datagrams. Packing is only done on loopback: a remote DogStatsD may
// read with a smaller buffer, and a lost fragment would drop the whole packet.
func dogstatsdProxyPacketSize(addr *net.UDPAddr, bufferSize int) int {
	if !addr.IP.IsLoopback() && !addr.IP.IsUnspecified() {
		return 0
	}
	return max(min(bufferSize, maxDogstatsdProxyPacketSize), 0)
}

// dogstatsdProxyHandler returns a new HTTP handler which will proxy requests to
// the DogStatsD endpoint in the Core Agent over UDP. Communication between the
// proxy and the agent does not support UDS (see #13628), and so does not guarantee delivery of
// all statsd payloads.
//
// On loopback, lines are packed into datagrams (see dogstatsdProxyPacketSize):
// a burst of small datagrams fills the receive buffer much faster than the same
// bytes in a few larger ones. Lines longer than the packet are sent alone.
//
// The request body is relayed as it is read, so a body that turns out to be
// unreadable, over the size limit, or over maxDogstatsdProxyLines is
// reported as an error only after its earlier payloads have been sent. A client
// that retries such a request may therefore submit those payloads twice.
func (r *HTTPReceiver) dogstatsdProxyHandler() http.Handler {
	if !r.conf.StatsdEnabled {
		log.Info("DogstatsD disabled in the Agent configuration. The DogstatsD proxy endpoint will be non-functional.")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "503 Status Unavailable", http.StatusServiceUnavailable)
		})
	}
	if r.conf.StatsdPort == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Agent dogstatsd UDP port not configured, but required for dogstatsd proxy.", http.StatusServiceUnavailable)
		})
	}
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(r.conf.StatsdHost, strconv.Itoa(r.conf.StatsdPort)))
	if err != nil {
		log.Errorf("Error resolving dogstatsd proxy addr to %s endpoint at %q: %v", "udp", addr, err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Failed to resolve dogstatsd address", http.StatusInternalServerError)
		})
	}
	// With a packet size of 0, every line is sent alone.
	packetSize := dogstatsdProxyPacketSize(addr, r.conf.StatsdBufferSize)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Body = apiutil.NewLimitedReader(req.Body, r.conf.MaxRequestBytes)
		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			log.Errorf("Error connecting to %s endpoint at %q: %v", "udp", addr, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		// Scan the body one line at a time rather than splitting it up front:
		// splitting materializes a slice header per newline byte, so a body made
		// of newlines multiplies its own size in memory several times over. The
		// scanner keeps the extra memory proportional to the longest line.
		scanner := bufio.NewScanner(req.Body)
		// Grown on demand, so short bodies stay cheap.
		var packet []byte
		// flush sends the lines packed so far, if any, as one datagram.
		flush := func() error {
			if len(packet) == 0 {
				return nil
			}
			_, err := conn.Write(packet)
			packet = packet[:0]
			return err
		}
		lines := 0
		for scanner.Scan() {
			lines++
			if lines > maxDogstatsdProxyLines {
				log.Errorf("Dogstatsd proxy request contains more than %d lines, dropping the rest.", maxDogstatsdProxyLines)
				// A send error takes precedence.
				if err := flush(); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				http.Error(w, "too many dogstatsd payloads in request", http.StatusRequestEntityTooLarge)
				return
			}
			payload := scanner.Bytes()
			if len(payload) == 0 {
				// Nothing for DogStatsD to parse; don't spend a syscall on it.
				continue
			}
			// Flush if this line and its separator would not fit.
			if len(packet) > 0 && len(packet)+1+len(payload) > packetSize {
				if err := flush(); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			if len(payload) > packetSize {
				// Too long to pack; send it alone.
				if _, err := conn.Write(payload); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				continue
			}
			if len(packet) > 0 {
				packet = append(packet, '\n')
			}
			// Copied, as the scanner reuses its buffer.
			packet = append(packet, payload...)
		}
		// Relay what was read before reporting a read error.
		if err := flush(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := scanner.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}
