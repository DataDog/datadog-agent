package tcp

import (
	"context"
	"encoding/binary"
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
	// URG is the urgent TCP flag
	URG = 1 << 5
	// ACK is the acknowledge TCP flag
	ACK = 1 << 4
	// PSH is the push TCP flag
	PSH = 1 << 3
	// RST is the reset TCP flag
	RST = 1 << 2
	// SYN is the synchronization TCP flag
	SYN = 1 << 1
	// FIN is the final TCP flag
	FIN = 1 << 0
)

type (
	// CanceledError is sent when a listener
	// is canceled
	CanceledError string

	// ICMPResponse encapsulates the data from
	// an ICMP response packet needed for matching
	ICMPResponse struct {
		SrcIP        net.IP
		DstIP        net.IP
		TypeCode     layers.ICMPv4TypeCode
		InnerSrcIP   net.IP
		InnerDstIP   net.IP
		InnerSrcPort uint16
		InnerDstPort uint16
		InnerSeqNum  uint32
	}

	// TCPResponse encapsulates the data from a
	// TCP response needed for matching
	TCPResponse struct {
		SrcIP       net.IP
		DstIP       net.IP
		TCPResponse *layers.TCP
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

// readRawPacket creates a gopacket given a byte array
// containing a packet
//
// TODO: try doing this manually to see if it's more performant
// we should either always use gopacket or never use gopacket
func readRawPacket(rawPacket []byte) gopacket.Packet {
	return gopacket.NewPacket(rawPacket, layers.LayerTypeIPv4, gopacket.Default)
}

// LayerCat prints the IPv4, TCP, and ICMP layers of
// a packet then lists all layers by type
func LayerCat(pkt gopacket.Packet) error {
	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		fmt.Println("This is an IPv4 packet!")
		tcp, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return fmt.Errorf("failed to assert IPv4 layer type")
		}
		fmt.Printf("IPv4 layer: %+v\n", tcp)
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		fmt.Println("This is a TCP packet!")
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return fmt.Errorf("failed to assert TCP layer type")
		}
		fmt.Printf("TCP layer: %+v\n", tcp)
	}

	if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
		fmt.Println("This is an ICMPv4 packet!")
		tcp, ok := icmpLayer.(*layers.ICMPv4)
		if !ok {
			return fmt.Errorf("failed to assert ICMPv4 layer type")
		}
		fmt.Printf("ICMPv4 layer: %+v\n", tcp)
	}

	for _, layer := range pkt.Layers() {
		fmt.Println("Packet layer: ", layer.LayerType())
	}

	return nil
}

// ParseICMPPacket takes in a gopacket.Packet and tries to convert to an ICMP message
// it returns all the fields from the packet we need to validate it's the response
// we're looking for
func ParseICMPPacket(pkt gopacket.Packet) (*ICMPResponse, error) {
	// this parsing could likely be improved to be more performant if we read from the
	// the original packet bytes directly where we expect the required fields to be
	// or even just creating a single DecodingLayerParser but in both cases we lose
	// some flexibility
	icmpResponse := ICMPResponse{}

	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return nil, fmt.Errorf("failed to assert IPv4 layer type")
		}

		icmpResponse.SrcIP = ip.SrcIP
		icmpResponse.DstIP = ip.DstIP
	}

	if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
		icmp, ok := icmpLayer.(*layers.ICMPv4)
		if !ok {
			return nil, fmt.Errorf("failed to assert ICMPv4 layer type")
		}
		icmpResponse.TypeCode = icmp.TypeCode

		var payload []byte
		if len(icmp.Payload) < 40 {
			log.Debugf("Payload length %d is less than 40, extending...\n", len(icmp.Payload))
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
	}

	return &icmpResponse, nil
}

func ParseTCPPacket(pkt gopacket.Packet) (*TCPResponse, error) {
	// this parsing could likely be improved to be more performant if we read from the
	// the original packet bytes directly where we expect the required fields to be
	tcpResponse := TCPResponse{}

	// TODO: separate this out into its own function since we do this
	// for ICMP and TCP
	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return nil, fmt.Errorf("failed to assert IPv4 layer type")
		}

		tcpResponse.SrcIP = ip.SrcIP
		tcpResponse.DstIP = ip.DstIP
	} else {
		return nil, fmt.Errorf("packet does not contain an IPv4 layer")
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return nil, fmt.Errorf("failed to assert TCP layer type")
		}

		tcpResponse.TCPResponse = tcp
	} else {
		return nil, fmt.Errorf("packet does not contain an TCP layer")
	}

	return &tcpResponse, nil
}

// CreateRawTCPPacket creates a TCP packet with the specified parameters
func CreateRawTCPPacket(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int, flags byte) (*ipv4.Header, []byte, error) {
	ipHdr := ipv4.Header{
		Version:  4,
		Len:      20,
		TTL:      ttl,
		ID:       418218,
		Protocol: 6, // TCP
		Dst:      destIP,
		Src:      sourceIP,
	}

	// Create TCP packet with the specified flags
	// and sequence number
	tcpPacket := make([]byte, 20)
	binary.BigEndian.PutUint16(tcpPacket[0:2], sourcePort) // source port
	binary.BigEndian.PutUint16(tcpPacket[2:4], destPort)   // destination port
	binary.BigEndian.PutUint32(tcpPacket[4:8], seqNum)     // sequence number
	binary.BigEndian.PutUint32(tcpPacket[8:12], 0)         // ack number
	tcpPacket[12] = 5 << 4                                 // header length
	tcpPacket[13] = flags
	binary.BigEndian.PutUint16(tcpPacket[14:16], 1024) // window size

	cs := tcpChecksum(&ipHdr, tcpPacket)
	binary.BigEndian.PutUint16(tcpPacket[16:18], cs) // checksum

	return &ipHdr, tcpPacket, nil
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

// SendPacket sends a raw IPv4 packet using the passed connection
func SendPacket(rawConn *ipv4.RawConn, header *ipv4.Header, payload []byte) error {
	if err := rawConn.WriteTo(header, payload, nil); err != nil {
		return err
	}

	return nil
}

func checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	return uint16(^sum)
}

func tcpChecksum(ipHdr *ipv4.Header, tcpHeader []byte) uint16 {
	pseudoHeader := []byte{}
	pseudoHeader = append(pseudoHeader, ipHdr.Src.To4()...)
	pseudoHeader = append(pseudoHeader, ipHdr.Dst.To4()...)
	pseudoHeader = append(pseudoHeader, 0) // reserved
	pseudoHeader = append(pseudoHeader, byte(ipHdr.Protocol))
	pseudoHeader = append(pseudoHeader, 0, byte(len(tcpHeader))) // tcp length

	return checksum(append(pseudoHeader, tcpHeader...))
}

func listenAnyPacket(icmpConn *ipv4.RawConn, tcpConn *ipv4.RawConn, timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, time.Time, error) {
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
		_, tcpCanceled := tcpErr.(CanceledError)
		_, icmpCanceled := icmpErr.(CanceledError)
		if icmpCanceled && tcpCanceled {
			// TODO: better handling of listener timeout case
			// this signifies an unknown hop
			//
			// TODO: better handling of the mismatch case which
			// right now becomes an unknown hop
			//
			// For now, return nil error with empty hop data
			log.Debug("timed out waiting for responses")
			return net.IP{}, 0, 0, finished, nil
		}
		log.Errorf("TCP Error: %s", tcpErr.Error())
		log.Errorf("ICMP Error: %s", icmpErr.Error())
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
// timeout, it should return a timeout exceeded error
func handlePackets(ctx context.Context, conn *ipv4.RawConn, listener string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, layers.ICMPv4TypeCode, error) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return net.IP{}, 0, 0, CanceledError("listener canceled")
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
				return net.IP{}, 0, 0, err
			}
		}
		// TODO: remove listener constraint and parse all packets
		// in the same function return a succinct struct here
		if listener == "icmp" {
			icmpResponse, err := parseICMP(header, packet)
			if err != nil {
				return net.IP{}, 0, 0, fmt.Errorf("failed to parse ICMP packet: %w", err)
			}
			if icmpMatch(localIP, localPort, remoteIP, remotePort, seqNum, icmpResponse) {
				return icmpResponse.SrcIP, 0, icmpResponse.TypeCode, nil
			}
		} else if listener == "tcp" {
			tcpResp, err := parseTCP(header, packet)
			if err != nil {
				return net.IP{}, 0, 0, fmt.Errorf("failed to parse TCP packet: %w", err)
			}
			if tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
				return tcpResp.SrcIP, uint16(tcpResp.TCPResponse.SrcPort), 0, nil
			}
		} else {
			return net.IP{}, 0, 0, fmt.Errorf("unsupported listener type")
		}
	}
}

func parseICMP(header *ipv4.Header, payload []byte) (*ICMPResponse, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet: %w", err)
	}
	packet := readRawPacket(packetBytes)

	return ParseICMPPacket(packet)
}

func icmpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, icmpResponse *ICMPResponse) bool {
	log.Debugf("Sent packet fields SRC %s:%d, DST %s:%d, Seq: %d", localIP.String(), localPort, remoteIP.String(), remotePort, seqNum)
	log.Debugf("Received ICMP fields SRC %s:%d, DST %s:%d, Seq: %d", icmpResponse.InnerSrcIP.String(), icmpResponse.InnerSrcPort, icmpResponse.InnerDstIP.String(), icmpResponse.InnerDstPort, icmpResponse.InnerSeqNum)
	return localIP.Equal(icmpResponse.InnerSrcIP) &&
		remoteIP.Equal(icmpResponse.InnerDstIP) &&
		localPort == icmpResponse.InnerSrcPort &&
		remotePort == icmpResponse.InnerDstPort &&
		seqNum == icmpResponse.InnerSeqNum
}

func parseTCP(header *ipv4.Header, payload []byte) (*TCPResponse, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet: %w", err)
	}

	packet := readRawPacket(packetBytes)
	tcpResp, err := ParseTCPPacket(packet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TCP packet: %w", err)
	}

	return tcpResp, nil
}

func tcpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, tcpResponse *TCPResponse) bool {
	flagsCheck := (tcpResponse.TCPResponse.SYN && tcpResponse.TCPResponse.ACK) || tcpResponse.TCPResponse.RST
	sourcePort := uint16(tcpResponse.TCPResponse.SrcPort)
	destPort := uint16(tcpResponse.TCPResponse.DstPort)

	return remoteIP.Equal(tcpResponse.SrcIP) &&
		remotePort == sourcePort &&
		localIP.Equal(tcpResponse.DstIP) &&
		localPort == destPort &&
		seqNum == tcpResponse.TCPResponse.Ack-1 &&
		flagsCheck
}

func (c CanceledError) Error() string {
	return string(c)
}
