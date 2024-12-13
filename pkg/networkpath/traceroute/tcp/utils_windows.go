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
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	recvFrom          = windows.Recvfrom
	handlePacketsFunc = handlePackets
)

// listenPackets takes in raw ICMP and TCP connections and listens for matching ICMP
// and TCP responses based on the passed in trace information. If neither listener
// receives a matching packet within the timeout, a blank response is returned.
// Once a matching packet is received by a listener, it will cause the other listener
// to be canceled, and data from the matching packet will be returned to the caller
func listenPackets(w *common.Winrawsocket, timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ip, finished, err := handlePacketsFunc(ctx, w, localIP, localPort, remoteIP, remotePort, seqNum)
	if err != nil {
		_, canceled := err.(common.CanceledError)
		if canceled {
			log.Trace("timed out waiting for responses")
			return net.IP{}, time.Time{}, nil
		}
		log.Errorf("listener error: %s", err.Error())

		return net.IP{}, time.Time{}, fmt.Errorf("error: %w", err)
	}

	// return the response
	return ip, finished, nil
}

// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout or if the listener is canceled, it should return a canceledError
func handlePackets(ctx context.Context, w *common.Winrawsocket, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, time.Time, error) {
	buf := make([]byte, 512)
	tp := newTCPParser()
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, time.Time{}, common.CanceledError("listener canceled")
		default:
		}

		// the receive timeout is set to 100ms in the constructor, to match the
		// linux side. This is a workaround for the lack of a deadline for sockets.
		//err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
		n, _, err := recvFrom(w.Socket, buf, 0)
		if err != nil {
			if err == windows.WSAETIMEDOUT {
				continue
			}
			if err == windows.WSAEMSGSIZE {
				log.Warnf("Message too large for buffer")
				continue
			}
			return nil, time.Time{}, err
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
		if header.Protocol == windows.IPPROTO_ICMP {
			icmpResponse, err := common.ParseICMP(header, packet)
			if err != nil {
				log.Tracef("failed to parse ICMP packet: %s", err.Error())
				continue
			}
			if common.ICMPMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, received, nil
			}
		} else if header.Protocol == windows.IPPROTO_TCP {
			// don't even bother parsing the packet if the src/dst ip don't match
			if !localIP.Equal(header.Dst) || !remoteIP.Equal(header.Src) {
				continue
			}
			tcpResp, err := tp.parseTCP(header, packet)
			if err != nil {
				log.Tracef("failed to parse TCP packet: %s", err.Error())
				continue
			}
			if tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
				return tcpResp.SrcIP, received, nil
			}
		} else {
			continue
		}
	}
}
