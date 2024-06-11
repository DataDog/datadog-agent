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

const (
	// ACK is the acknowledge TCP flag
	ACK = 1 << 4
	// RST is the reset TCP flag
	RST = 1 << 2
	// SYN is the synchronization TCP flag
	SYN = 1 << 1
)

type (
	// canceledError is sent when a listener
	// is canceled
	canceledError string

	// icmpResponse encapsulates the data from
	// an ICMP response packet needed for matching
	icmpResponse struct {
		SrcIP        net.IP
		DstIP        net.IP
		TypeCode     layers.ICMPv4TypeCode
		InnerSrcIP   net.IP
		InnerDstIP   net.IP
		InnerSrcPort uint16
		InnerDstPort uint16
		InnerSeqNum  uint32
	}

	// tcpResponse encapsulates the data from a
	// TCP response needed for matching
	tcpResponse struct {
		SrcIP       net.IP
		DstIP       net.IP
		TCPResponse *layers.TCP
	}

	rawConnWrapper interface {
		SetReadDeadline(t time.Time) error
		ReadFrom(b []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error)
		WriteTo(h *ipv4.Header, p []byte, cm *ipv4.ControlMessage) error
	}
)

func localAddrForHost(destIP net.IP, destPort uint16) (*net.UDPAddr, error) {
	// this is a quick way to get the local address for connecting to the host
	// using UDP as the network type to avoid actually creating a connection to
	// the host, just get the OS to give us a local IP and local ephemeral port
	conn, err := net.Dial("udp4", net.JoinHostPort(destIP.String(), strconv.Itoa(int(destPort))))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr()

	localUDPAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localUDPAddr, localAddr)
	}

	return localUDPAddr, nil
}

// createRawTCPSyn creates a TCP packet with the specified parameters
func createRawTCPSyn(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int) (*ipv4.Header, []byte, error) {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      uint8(ttl),
		Id:       uint16(41821),
		Protocol: 6,
		DstIP:    destIP,
		SrcIP:    sourceIP,
	}

	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(sourcePort),
		DstPort: layers.TCPPort(destPort),
		Seq:     seqNum,
		Ack:     0,
		SYN:     true,
		Window:  1024,
	}

	err := tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create packet checksum: %w", err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err = gopacket.SerializeLayers(buf, opts,
		ipLayer,
		tcpLayer,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize packet: %w", err)
	}
	packet := buf.Bytes()

	var ipHdr ipv4.Header
	if err := ipHdr.Parse(packet[:20]); err != nil {
		return nil, nil, fmt.Errorf("failed to parse IP header: %w", err)
	}

	return &ipHdr, packet[20:], nil
}

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
	var port uint16
	wg.Add(2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		defer wg.Done()
		defer cancel()
		tcpIP, port, _, tcpErr = handlePackets(ctx, tcpConn, "tcp", localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		icmpIP, _, icmpCode, icmpErr = handlePackets(ctx, icmpConn, "icmp", localIP, localPort, remoteIP, remotePort, seqNum)
	}()
	wg.Wait()
	// TODO: while this is okay, we
	// should do this more cleanly
	finished := time.Now()

	if tcpErr != nil && icmpErr != nil {
		_, tcpCanceled := tcpErr.(canceledError)
		_, icmpCanceled := icmpErr.(canceledError)
		if icmpCanceled && tcpCanceled {
			log.Trace("timed out waiting for responses")
			return net.IP{}, 0, 0, finished, nil
		}
		if tcpErr != nil {
			log.Errorf("TCP listener error: %s", tcpErr.Error())
		}
		if icmpErr != nil {
			log.Errorf("ICMP listener error: %s", icmpErr.Error())
		}

		return net.IP{}, 0, 0, finished, multierr.Append(fmt.Errorf("tcp error: %w", tcpErr), fmt.Errorf("icmp error: %w", icmpErr))
	}

	// if there was an error for TCP, but not
	// ICMP, return the ICMP response
	if tcpErr != nil {
		return icmpIP, port, icmpCode, finished, nil
	}

	// return the TCP response
	return tcpIP, port, 0, finished, nil
}

// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout or if the listener is canceled, it should return a canceledError
func handlePackets(ctx context.Context, conn rawConnWrapper, listener string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, error) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, 0, 0, canceledError("listener canceled")
		default:
		}
		now := time.Now()
		err := conn.SetReadDeadline(now.Add(time.Millisecond * 100))
		if err != nil {
			return net.IP{}, 0, 0, fmt.Errorf("failed to read: %w", err)
		}
		header, packet, _, err := conn.ReadFrom(buf)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok {
				if nerr.Timeout() {
					continue
				}
			}
			return net.IP{}, 0, 0, err
		}
		// TODO: remove listener constraint and parse all packets
		// in the same function return a succinct struct here
		if listener == "icmp" {
			icmpResponse, err := parseICMP(header, packet)
			if err != nil {
				log.Debugf("failed to parse ICMP packet: %s", err.Error())
				continue
			}
			if icmpMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, 0, icmpResponse.TypeCode, nil
			}
		} else if listener == "tcp" {
			tcpResp, err := parseTCP(header, packet)
			if err != nil {
				log.Debugf("failed to parse TCP packet: %s", err.Error())
				continue
			}
			if tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
				return tcpResp.SrcIP, uint16(tcpResp.TCPResponse.SrcPort), 0, nil
			}
		} else {
			return net.IP{}, 0, 0, fmt.Errorf("unsupported listener type")
		}
	}
}

// MarshalPacket takes in an ipv4 header and a payload and copies
// them into a newly allocated []byte
func MarshalPacket(header *ipv4.Header, payload []byte) ([]byte, error) {
	hdrBytes, err := header.Marshal()
	if err != nil {
		return nil, err
	}

	packet := make([]byte, len(hdrBytes)+len(payload))
	copy(packet[:len(hdrBytes)], hdrBytes)
	copy(packet[len(hdrBytes):], payload)

	return packet, nil
}

// readRawPacket creates a gopacket given a byte array
// containing a packet
//
// TODO: try doing this manually to see if it's more performant
// we should either always use gopacket or never use gopacket
func readRawPacket(rawPacket []byte) gopacket.Packet {
	return gopacket.NewPacket(rawPacket, layers.LayerTypeIPv4, gopacket.Default)
}

func parseIPv4Layer(pkt gopacket.Packet) (net.IP, net.IP, error) {
	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return net.IP{}, net.IP{}, fmt.Errorf("failed to assert IPv4 layer type")
		}

		return ip.SrcIP, ip.DstIP, nil
	}

	return net.IP{}, net.IP{}, fmt.Errorf("packet does not contain an IPv4 layer")
}

func parseICMP(header *ipv4.Header, payload []byte) (*icmpResponse, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet: %w", err)
	}
	packet := readRawPacket(packetBytes)

	return parseICMPPacket(packet)
}

// parseICMPPacket takes in a gopacket.Packet and tries to convert to an ICMP message
// it returns all the fields from the packet we need to validate it's the response
// we're looking for
func parseICMPPacket(pkt gopacket.Packet) (*icmpResponse, error) {
	// this parsing could likely be improved to be more performant if we read from the
	// the original packet bytes directly where we expect the required fields to be
	// or even just creating a single DecodingLayerParser but in both cases we lose
	// some flexibility
	var err error
	icmpResponse := icmpResponse{}

	icmpResponse.SrcIP, icmpResponse.DstIP, err = parseIPv4Layer(pkt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}

	if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
		icmp, ok := icmpLayer.(*layers.ICMPv4)
		if !ok {
			return nil, fmt.Errorf("failed to assert ICMPv4 layer type")
		}
		icmpResponse.TypeCode = icmp.TypeCode

		var payload []byte
		if len(icmp.Payload) < 40 {
			log.Tracef("Payload length %d is less than 40, extending...\n", len(icmp.Payload))
			payload = make([]byte, 40)
			copy(payload, icmp.Payload)
			// we have to set this in order for the TCP
			// parser to work
			payload[32] = 5 << 4 // set data offset
		} else {
			payload = icmp.Payload
		}

		// if we're in an ICMP packet, we know that we should have
		// an inner IPv4 and TCP header section
		var innerIPLayer layers.IPv4
		var innerTCPLayer layers.TCP
		decoded := []gopacket.LayerType{}
		innerIPParser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &innerIPLayer, &innerTCPLayer)
		if err := innerIPParser.DecodeLayers(payload, &decoded); err != nil {
			return nil, fmt.Errorf("failed to decode ICMP payload: %w", err)
		}
		icmpResponse.InnerSrcIP = innerIPLayer.SrcIP
		icmpResponse.InnerDstIP = innerIPLayer.DstIP
		icmpResponse.InnerSrcPort = uint16(innerTCPLayer.SrcPort)
		icmpResponse.InnerDstPort = uint16(innerTCPLayer.DstPort)
		icmpResponse.InnerSeqNum = innerTCPLayer.Seq
	} else {
		return nil, fmt.Errorf("packet does not contain an ICMP layer")
	}

	return &icmpResponse, nil
}

func parseTCP(header *ipv4.Header, payload []byte) (*tcpResponse, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet: %w", err)
	}

	packet := readRawPacket(packetBytes)
	tcpResp, err := parseTCPPacket(packet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TCP packet: %w", err)
	}

	return tcpResp, nil
}

func parseTCPPacket(pkt gopacket.Packet) (*tcpResponse, error) {
	// this parsing could likely be improved to be more performant if we read from the
	// the original packet bytes directly where we expect the required fields to be
	var err error
	tcpResponse := tcpResponse{}

	tcpResponse.SrcIP, tcpResponse.DstIP, err = parseIPv4Layer(pkt)
	if err != nil {
		return nil, err
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return nil, fmt.Errorf("failed to assert TCP layer type")
		}

		tcpResponse.TCPResponse = tcp
	} else {
		return nil, fmt.Errorf("packet does not contain a TCP layer")
	}

	return &tcpResponse, nil
}

func icmpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, response *icmpResponse) bool {
	return localIP.Equal(response.InnerSrcIP) &&
		remoteIP.Equal(response.InnerDstIP) &&
		localPort == response.InnerSrcPort &&
		remotePort == response.InnerDstPort &&
		seqNum == response.InnerSeqNum
}

func tcpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, response *tcpResponse) bool {
	flagsCheck := (response.TCPResponse.SYN && response.TCPResponse.ACK) || response.TCPResponse.RST
	sourcePort := uint16(response.TCPResponse.SrcPort)
	destPort := uint16(response.TCPResponse.DstPort)

	return remoteIP.Equal(response.SrcIP) &&
		remotePort == sourcePort &&
		localIP.Equal(response.DstIP) &&
		localPort == destPort &&
		seqNum == response.TCPResponse.Ack-1 &&
		flagsCheck
}

func (c canceledError) Error() string {
	return string(c)
}
