// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package tcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/icmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/multierr"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

type (
	packetResponse struct {
		IP   net.IP
		Type uint8
		Code uint8
		Port uint16
		Time time.Time
		Err  error
	}
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
func listenPackets(icmpConn rawConnWrapper, tcpConn rawConnWrapper, timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) packetResponse {
	respChan := make(chan packetResponse, 2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		respChan <- handlePackets(ctx, tcpConn, localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	go func() {
		respChan <- handlePackets(ctx, icmpConn, localIP, localPort, remoteIP, remotePort, seqNum)
	}()

	// wait for both responses to return
	// as one could error even if the other
	// succeeds
	var err error
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			log.Trace("timed out waiting for responses")
			return packetResponse{
				Err: err,
			}
		case resp := <-respChan:
			if resp.Err == nil {
				return resp
			}

			// avoid adding canceled errors to the error list
			// TODO: maybe just return nil on timeout?
			if _, isCanceled := resp.Err.(common.CanceledError); !isCanceled {
				err = multierr.Append(err, resp.Err)
			}
		}
	}

	return packetResponse{
		Err: err,
	}
}

// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout or if the listener is canceled, it should return a canceledError
func handlePackets(ctx context.Context, conn rawConnWrapper, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) packetResponse {
	buf := make([]byte, 1024)
	tp := newParser()
	icmpParser := icmp.NewICMPTCPParser()
	for {
		select {
		case <-ctx.Done():
			return packetResponse{
				Err: common.CanceledError("listener canceled"),
			}
		default:
		}
		now := time.Now()
		err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
		if err != nil {
			return packetResponse{
				Err: fmt.Errorf("failed to set read deadline: %w", err),
			}
		}
		header, packet, _, err := conn.ReadFrom(buf)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok {
				if nerr.Timeout() {
					continue
				}
			}
			return packetResponse{
				Err: err,
			}
		}
		// once we have a packet, take a timestamp to know when
		// the response was received, if it matches, we will
		// return this timestamp
		received := time.Now()

		if header.Protocol == unix.IPPROTO_ICMP {
			icmpResponse, err := icmpParser.Parse(header, packet)
			if err != nil {
				log.Tracef("failed to parse ICMP packet: %s", err)
				continue
			}
			if icmpResponse.Matches(localIP, localPort, remoteIP, remotePort, seqNum) {
				return packetResponse{
					IP:   icmpResponse.SrcIP,
					Type: icmpResponse.TypeCode.Type(),
					Code: icmpResponse.TypeCode.Code(),
					Time: received,
				}
			}
		} else if header.Protocol == unix.IPPROTO_TCP {
			tcpResp, err := tp.parseTCP(header, packet)
			if err != nil {
				log.Tracef("failed to parse TCP packet: %s", err)
				continue
			}
			if tcpResp.Match(localIP, localPort, remoteIP, remotePort, seqNum) {
				return packetResponse{
					IP:   tcpResp.SrcIP,
					Port: uint16(tcpResp.SrcPort),
					Time: received,
				}
			}
		}
	}
}
