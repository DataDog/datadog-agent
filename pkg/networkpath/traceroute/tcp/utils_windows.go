// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp


import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
)


// listenPackets takes in raw ICMP and TCP connections and listens for matching ICMP
// and TCP responses based on the passed in trace information. If neither listener
// receives a matching packet within the timeout, a blank response is returned.
// Once a matching packet is received by a listener, it will cause the other listener
// to be canceled, and data from the matching packet will be returned to the caller
func (w *winrawsocket) listenPackets(timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, time.Time, error) {
	var icmpErr error
	var wg sync.WaitGroup
	var icmpIP net.IP
	//var tcpIP net.IP
	//var icmpCode layers.ICMPv4TypeCode
	//var tcpFinished time.Time
	var icmpFinished time.Time
	var port uint16
	wg.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		defer wg.Done()
		defer cancel()
		icmpIP, _, _, icmpFinished, icmpErr = w.handlePackets(ctx, localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	wg.Wait()

	if icmpErr != nil {
		_, icmpCanceled := icmpErr.(canceledError)
		if icmpCanceled {
			log.Trace("timed out waiting for responses")
			return net.IP{}, 0, 0, time.Time{}, nil
		}
		if icmpErr != nil {
			log.Errorf("ICMP listener error: %s", icmpErr.Error())
		}

		return net.IP{}, 0, 0, time.Time{}, fmt.Errorf("icmp error: %w", icmpErr)
	}


	// return the TCP response
	return icmpIP, port, 0, icmpFinished, nil
}


// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout or if the listener is canceled, it should return a canceledError
func (w* winrawsocket) handlePackets(ctx context.Context, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, time.Time, error) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, 0, 0, time.Time{}, canceledError("listener canceled")
		default:
		}
		
		// the receive timeout is set to 100ms in the constructor, to match the
		// linux side. This is a workaround for the lack of a deadline for sockets.
		//err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
		n, _, err := windows.Recvfrom(w.s, buf, 0)
		if err != nil {
			if err == windows.WSAETIMEDOUT {
				continue;
			}
			return nil, 0, 0, time.Time{}, err
		}
		log.Tracef("Got packet %+v", buf[:n])

		if n < 20 { // min size of ipv4 header
			continue
		}
		header, err := ipv4.ParseHeader(buf[:n])
		if err != nil {
			continue
		}
		packet := buf[header.Len:header.TotalLen]
		
		// once we have a packet, take a timestamp to know when
		// the response was received, if it matches, we will
		// return this timestamp
		received := time.Now()
		// TODO: remove listener constraint and parse all packets
		// in the same function return a succinct struct here
		if header.Protocol == windows.IPPROTO_ICMP {
			icmpResponse, err := parseICMP(header, packet)
			if err != nil {
				log.Tracef("failed to parse ICMP packet: %s", err.Error())
				continue
			}
			if icmpMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, 0, icmpResponse.TypeCode, received, nil
			}
		} else if header.Protocol == windows.IPPROTO_TCP {
			// don't even bother parsing the packet if the src/dst ip don't match
			if !localIP.Equal(header.Dst) || !remoteIP.Equal(header.Src) {
				continue
			}
			tcpResp, err := parseTCP(header, packet)
			if err != nil {
				log.Tracef("failed to parse TCP packet: %s", err.Error())
				continue
			}
			if tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
				return tcpResp.SrcIP, uint16(tcpResp.TCPResponse.SrcPort), 0, received, nil
			}
		} else {
			continue
		}
	}
}
