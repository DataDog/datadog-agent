// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package winconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

type (
	// PCAPRawConn is a struct that encapsulates a raw socket
	// on Windows that can be used to listen to traffic on a host
	// or send raw packets from a host
	PCAPRawConn struct {
		Socket     windows.Handle
		pcapHandle *pcap.Handle
	}
)

// Close closes the raw socket
func (r *PCAPRawConn) Close() {
	if r.Socket != windows.InvalidHandle {
		windows.Closesocket(r.Socket) // nolint: errcheck
	}
	r.Socket = windows.InvalidHandle
}

// ReadFrom reads from the RawConn into the passed []byte and returns
// the IPv4 header and payload separately
func (r *PCAPRawConn) ReadFrom(b []byte) (*ipv4.Header, []byte, error) {
	// the receive timeout is set to 100ms in the constructor, to match the
	// linux side. This is a workaround for the lack of a deadline for sockets.
	//err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
	n, _, err := recvFrom(r.Socket, b, 0)
	if err != nil {
		return nil, nil, err
	}
	log.Tracef("Got packet %+v", b[:n])

	if n < 20 { // min size of ipv4 header
		return nil, nil, errors.New("packet too small to be an IPv4 packet")
	}
	header, err := ipv4.ParseHeader(b[:n])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse IPv4 header: %w", err)
	}

	return header, b[header.Len:header.TotalLen], nil
}

// NewRawConn creates a Winrawsocket
func NewPCAPRawConn(iface string) (*PCAPRawConn, error) {
	s, err := windows.Socket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	on := int(1)
	err = windows.SetsockoptInt(s, windows.IPPROTO_IP, windows.IP_HDRINCL, on)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set IP_HDRINCL: %w", err)
	}

	err = windows.SetsockoptInt(s, windows.SOL_SOCKET, windows.SO_RCVTIMEO, 100)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set SO_RCVTIMEO: %w", err)
	}

	handle, err := pcap.OpenLive(iface, 512, true, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to start PCAP for interface %s: %w", iface, err)
	}

	return &PCAPRawConn{
		Socket:     s,
		pcapHandle: handle,
	}, nil
}

// SendRawPacket sends a raw packet to a destination IP and port
func (r *PCAPRawConn) SendRawPacket(destIP net.IP, destPort uint16, payload []byte) error {

	dst := destIP.To4()
	if dst == nil {
		return errors.New("unable to parse IP address")
	}
	sa := &windows.SockaddrInet4{
		Port: int(destPort),
		Addr: [4]byte{dst[0], dst[1], dst[2], dst[3]},
	}
	if err := sendTo(r.Socket, payload, 0, sa); err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}
	return nil
}

// ListenPackets listens for matching responses based on the passed in trace information and decoderFunc.
// If neither decoderFunc receives a matching packet within the timeout, a blank response is returned.
// Once a matching packet is received by a decoderFunc, it will cause the other decoderFuncs to be
// canceled, and data from the matching packet will be returned to the caller
func (r *PCAPRawConn) ListenPackets(timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, matcherFuncs map[int]common.MatcherFunc) (net.IP, time.Time, error) {
	// filter down to only ICMP and destination IP (might be able to do port later)
	err := r.pcapHandle.SetBPFFilter(fmt.Sprintf("host %s or icmp", remoteIP.String()))
	if err != nil {
		return net.IP{}, time.Time{}, fmt.Errorf("failed to set BPF filter: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ip, finished, err := r.handlePackets(ctx, localIP, localPort, remoteIP, remotePort, innerIdentifier, matcherFuncs)
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
func (r *PCAPRawConn) handlePackets(ctx context.Context, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, matcherFuncs map[int]common.MatcherFunc) (net.IP, time.Time, error) {
	// TODO: reset to 512 before merge?
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, time.Time{}, common.CanceledError("listener canceled")
		default:
		}

		//header, packet, err := r.ReadFrom(buf)
		packet, _, err := r.pcapHandle.ReadPacketData()
		if err != nil {
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			return nil, time.Time{}, err
		}
		log.Tracef("Got packet: %+v", packet)

		// 34 is length of Ethernet + minimum IPv4 header length
		if len(packet) < 34 {
			return nil, time.Time{}, fmt.Errorf("packet too short for IP header: length %d", len(packet))
		}

		header, err := ipv4.ParseHeader(packet[14:34])
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to read header: %w", err)
		}
		log.Tracef("Parsed header: %+v", header)

		// once we have a packet, take a timestamp to know when
		// the response was received, if it matches, we will
		// return this timestamp
		received := time.Now()
		matcherFunc, ok := matcherFuncs[header.Protocol]
		if !ok {
			continue
		}
		ip, err := matcherFunc(header, packet[34:], localIP, localPort, remoteIP, remotePort, innerIdentifier)
		if err != nil {
			// if packet is NOT a match continue, otherwise log
			// the error
			if _, ok := err.(common.MismatchError); !ok {
				log.Tracef("decoder error: %s", err.Error())
			} else {
				log.Tracef("mismatch error: %s", err.Error())
			}
			continue
		}
		return ip, received, nil
	}
}
