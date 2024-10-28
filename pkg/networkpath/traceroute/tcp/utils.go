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

	// IPProtoICMP is the ICMP protocol number
	IPProtoICMP = 1
	// IPProtoTCP is the TCP protocol number
	IPProtoTCP = 6
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
		TCPResponse layers.TCP
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
	tp := newTCPParser()
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
				log.Tracef("failed to parse ICMP packet: %s", err)
				continue
			}
			if icmpMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, 0, icmpResponse.TypeCode, received, nil
			}
		} else if listener == "tcp" {
			tcpResp, err := tp.parseTCP(header, packet)
			if err != nil {
				log.Tracef("failed to parse TCP packet: %s", err)
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

// parseICMP takes in an IPv4 header and payload and tries to convert to an ICMP
// message, it returns all the fields from the packet we need to validate it's the response
// we're looking for
func parseICMP(header *ipv4.Header, payload []byte) (*icmpResponse, error) {
	// in addition to parsing, it is probably not a bad idea to do some validation
	// so we can ignore the ICMP packets we don't care about
	icmpResponse := icmpResponse{}

	if header.Protocol != IPProtoICMP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for ICMP packet: %+v", header)
	}
	icmpResponse.SrcIP = header.Src
	icmpResponse.DstIP = header.Dst

	var icmpv4Layer layers.ICMPv4
	decoded := []gopacket.LayerType{}
	icmpParser := gopacket.NewDecodingLayerParser(layers.LayerTypeICMPv4, &icmpv4Layer)
	icmpParser.IgnoreUnsupported = true // ignore unsupported layers, we will decode them in the next step
	if err := icmpParser.DecodeLayers(payload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}
	// since we ignore unsupported layers, we need to check if we actually decoded
	// anything
	if len(decoded) < 1 {
		return nil, fmt.Errorf("failed to decode ICMP packet, no layers decoded")
	}
	icmpResponse.TypeCode = icmpv4Layer.TypeCode

	var icmpPayload []byte
	if len(icmpv4Layer.Payload) < 40 {
		log.Tracef("Payload length %d is less than 40, extending...\n", len(icmpv4Layer.Payload))
		icmpPayload = make([]byte, 40)
		copy(icmpPayload, icmpv4Layer.Payload)
		// we have to set this in order for the TCP
		// parser to work
		icmpPayload[32] = 5 << 4 // set data offset
	} else {
		icmpPayload = icmpv4Layer.Payload
	}

	// a separate parser is needed to decode the inner IP and TCP headers because
	// gopacket doesn't support this type of nesting in a single decoder
	var innerIPLayer layers.IPv4
	var innerTCPLayer layers.TCP
	innerIPParser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &innerIPLayer, &innerTCPLayer)
	if err := innerIPParser.DecodeLayers(icmpPayload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode inner ICMP payload: %w", err)
	}
	icmpResponse.InnerSrcIP = innerIPLayer.SrcIP
	icmpResponse.InnerDstIP = innerIPLayer.DstIP
	icmpResponse.InnerSrcPort = uint16(innerTCPLayer.SrcPort)
	icmpResponse.InnerDstPort = uint16(innerTCPLayer.DstPort)
	icmpResponse.InnerSeqNum = innerTCPLayer.Seq

	return &icmpResponse, nil
}

type tcpParser struct {
	layer               layers.TCP
	decoded             []gopacket.LayerType
	decodingLayerParser *gopacket.DecodingLayerParser
}

func newTCPParser() *tcpParser {
	tcpParser := &tcpParser{}
	tcpParser.decodingLayerParser = gopacket.NewDecodingLayerParser(layers.LayerTypeTCP, &tcpParser.layer)
	return tcpParser
}

func (tp *tcpParser) parseTCP(header *ipv4.Header, payload []byte) (*tcpResponse, error) {
	if header.Protocol != IPProtoTCP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for TCP packet: %+v", header)
	}

	if err := tp.decodingLayerParser.DecodeLayers(payload, &tp.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode TCP packet: %w", err)
	}

	resp := &tcpResponse{
		SrcIP:       header.Src,
		DstIP:       header.Dst,
		TCPResponse: tp.layer,
	}
	// make sure the TCP layer is cleared between runs
	tp.layer = layers.TCP{}

	return resp, nil
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
