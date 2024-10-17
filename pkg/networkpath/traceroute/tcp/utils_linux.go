// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"go.uber.org/multierr"
	"golang.org/x/net/ipv4"

)

// sendPacket sends a raw IPv4 packet using the passed connection
func sendPacket(rawConn rawConnWrapper, header *ipv4.Header, payload []byte) error {
	if err := rawConn.WriteTo(header, payload, nil); err != nil {
		return err
	}

	return nil
}


// listenPackets takes in raw ICMP and TCP connections and listens for matching ICMP
// and TCP responses based on the passed in trace information. If neither listener
// receives a matching packet within the timeout, a blank response is returned.
// Once a matching packet is received by a listener, it will cause the other listener
// to be canceled, and data from the matching packet will be returned to the caller
func listenPackets(icmpConn rawConnWrapper, tcpConn rawConnWrapper, timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, time.Time, error) {
	var tcpErr error
	var icmpErr error
	var wg sync.WaitGroup
	var icmpIP net.IP
	var tcpIP net.IP
	var icmpCode layers.ICMPv4TypeCode
	var tcpFinished time.Time
	var icmpFinished time.Time
	var port uint16
	wg.Add(2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		defer wg.Done()
		defer cancel()
		tcpIP, port, _, tcpFinished, tcpErr = handlePackets(ctx, tcpConn, "tcp", localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		icmpIP, _, icmpCode, icmpFinished, icmpErr = handlePackets(ctx, icmpConn, "icmp", localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	wg.Wait()

	if tcpErr != nil && icmpErr != nil {
		_, tcpCanceled := tcpErr.(canceledError)
		_, icmpCanceled := icmpErr.(canceledError)
		if icmpCanceled && tcpCanceled {
			log.Trace("timed out waiting for responses")
			return net.IP{}, 0, 0, time.Time{}, nil
		}
		if tcpErr != nil {
			log.Errorf("TCP listener error: %s", tcpErr.Error())
		}
		if icmpErr != nil {
			log.Errorf("ICMP listener error: %s", icmpErr.Error())
		}

		return net.IP{}, 0, 0, time.Time{}, multierr.Append(fmt.Errorf("tcp error: %w", tcpErr), fmt.Errorf("icmp error: %w", icmpErr))
	}

	// if there was an error for TCP, but not
	// ICMP, return the ICMP response
	if tcpErr != nil {
		return icmpIP, port, icmpCode, icmpFinished, nil
	}

	// return the TCP response
	return tcpIP, port, 0, tcpFinished, nil
}


// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout or if the listener is canceled, it should return a canceledError
func handlePackets(ctx context.Context, conn rawConnWrapper, listener string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, time.Time, error) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, 0, 0, time.Time{}, canceledError("listener canceled")
		default:
		}
		now := time.Now()
		err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
		if err != nil {
			return net.IP{}, 0, 0, time.Time{}, fmt.Errorf("failed to read: %w", err)
		}
		header, packet, _, err := conn.ReadFrom(buf)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok {
				if nerr.Timeout() {
					continue
				}
			}
			return net.IP{}, 0, 0, time.Time{}, err
		}
		// once we have a packet, take a timestamp to know when
		// the response was received, if it matches, we will
		// return this timestamp
		received := time.Now()
		// TODO: remove listener constraint and parse all packets
		// in the same function return a succinct struct here
		if listener == "icmp" {
			icmpResponse, err := parseICMP(header, packet)
			if err != nil {
				log.Tracef("failed to parse ICMP packet: %s", err.Error())
				continue
			}
			if icmpMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, 0, icmpResponse.TypeCode, received, nil
			}
		} else if listener == "tcp" {
			tcpResp, err := parseTCP(header, packet)
			if err != nil {
				log.Tracef("failed to parse TCP packet: %s", err.Error())
				continue
			}
			if tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
				return tcpResp.SrcIP, uint16(tcpResp.TCPResponse.SrcPort), 0, received, nil
			}
		} else {
			return net.IP{}, 0, 0, received, fmt.Errorf("unsupported listener type")
		}
	}
}
