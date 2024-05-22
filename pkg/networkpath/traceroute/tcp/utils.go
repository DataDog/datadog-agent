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
	URG = 1 << 5
	ACK = 1 << 4
	PSH = 1 << 3
	RST = 1 << 2
	SYN = 1 << 1
	FIN = 1 << 0
)

type (
	CanceledError string
	MismatchError string
)

func LocalAddrForHost(destIP net.IP, destPort uint16) (*net.UDPAddr, error) {
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

// ReadRawPacket creates a gopacket given a byte array
// containing a packet
//
// TODO: try doing this manually to see if it's more performant
// we should either always use gopacket or never use gopacket
func ReadRawPacket(rawPacket []byte) gopacket.Packet {
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

func ParseICMPPacket(pkt gopacket.Packet) (net.IP, net.IP, layers.ICMPv4TypeCode, error) {
	var src net.IP
	var dst net.IP
	var typeCode layers.ICMPv4TypeCode

	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		//fmt.Println("This is an IPv4 packet!")
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return net.IP{}, net.IP{}, layers.ICMPv4TypeCode(0), fmt.Errorf("failed to assert IPv4 layer type")
		}
		//fmt.Printf("IPv4 layer: %+v\n", ip)

		src = ip.SrcIP
		dst = ip.DstIP
	}

	if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
		//fmt.Println("This is an ICMPv4 packet!")
		icmp, ok := icmpLayer.(*layers.ICMPv4)
		if !ok {
			return net.IP{}, net.IP{}, layers.ICMPv4TypeCode(0), fmt.Errorf("failed to assert ICMPv4 layer type")
		}
		//fmt.Printf("ICMPv4 layer: %+v\n", icmp)
		typeCode = icmp.TypeCode
	}

	return src, dst, typeCode, nil
}

func ParseTCPPacket(pkt gopacket.Packet) (net.IP, net.IP, *layers.TCP, error) {
	var src net.IP
	var dst net.IP

	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		//fmt.Println("This is an IPv4 packet!")
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return net.IP{}, net.IP{}, nil, fmt.Errorf("failed to assert IPv4 layer type")
		}

		src = ip.SrcIP
		dst = ip.DstIP
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		//fmt.Println("This is a TCP packet!")
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return net.IP{}, net.IP{}, nil, fmt.Errorf("failed to assert TCP layer type")
		}

		return src, dst, tcp, nil
	}

	return src, dst, nil, fmt.Errorf("no tcp layer in packet")
}

// CreateRawTCPPacket creates a TCP packet with the specified parameters
func CreateRawTCPPacket(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int, flags byte) (*ipv4.Header, []byte, error) {
	ipHdr := ipv4.Header{
		Version:  4,
		Len:      20,
		TTL:      ttl,
		Protocol: 6, // TCP
		Dst:      destIP,
		Src:      sourceIP,
	}

	// Create TCP packet with the specified flags
	// we'll need to vary sequence number and do
	// some other manipulation for a paris-traceroute
	// like accuracy but for now let's just get a trace
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

	// TODO: calculate checksum

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
		} else {
			log.Errorf("TCP Error: %s", tcpErr.Error())
			log.Errorf("ICMP Error: %s", icmpErr.Error())
			return net.IP{}, 0, 0, finished, multierr.Append(fmt.Errorf("tcp error: %w", tcpErr), fmt.Errorf("icmp error: %w", icmpErr))
		}
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
		conn.SetReadDeadline(now.Add(time.Millisecond * 100))
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
		var ip net.IP
		var icmpCode layers.ICMPv4TypeCode
		var port uint16
		if listener == "icmp" {
			ip, icmpCode, err = parseICMP(header, packet)
			if err != nil {
				return net.IP{}, 0, 0, fmt.Errorf("failed to parse ICMP packet: %w", err)
			}
			log.Debugf("returning IP: %s", ip.String())
			return ip, 0, icmpCode, nil
		}
		if listener == "tcp" {
			ip, port, err = parseTCP(header, packet, localIP, localPort, remoteIP, remotePort, seqNum)
			if err != nil {
				_, ok := err.(MismatchError)
				if ok {
					continue
				}
				return net.IP{}, 0, 0, fmt.Errorf("failed to parse TCP packet: %w", err)
			}
			return ip, port, 0, nil
		}
	}
}

func parseICMP(header *ipv4.Header, payload []byte) (net.IP, layers.ICMPv4TypeCode, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to marshal packet: %w", err)
	}

	packet := ReadRawPacket(packetBytes)

	src, dst, icmpType, err := ParseICMPPacket(packet)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to parse ICMP packet: %w", err)
	}

	if icmpType == layers.ICMPv4TypeDestinationUnreachable || icmpType == layers.ICMPv4TypeTimeExceeded {
		log.Debugf("Received ICMP reply: %s from %s to %s", icmpType.String(), src.String(), dst.String())
	} else {
		log.Debugf("Received other ICMP reply: %s from %s to %s", icmpType.String(), src.String(), dst.String())
	}

	return src, icmpType, nil
}

func parseTCP(header *ipv4.Header, payload []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, uint16, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to marshal packet: %w", err)
	}

	packet := ReadRawPacket(packetBytes)
	source, dest, tcpLayer, err := ParseTCPPacket(packet)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to parse TCP packet: %w", err)
	}

	flagsCheck := tcpLayer.SYN && tcpLayer.ACK
	sourcePort := uint16(tcpLayer.SrcPort)
	destPort := uint16(tcpLayer.DstPort)

	// TODO: check flags, payload, sequence number here as well
	//
	// TODO: re-add sequence number check
	if source.Equal(remoteIP) && sourcePort == remotePort &&
		dest.Equal(localIP) && destPort == localPort &&
		flagsCheck {
		log.Debugf("Received MATCHING TCP Reply from: %s:%d with dest %s:%d\n", source.String(), sourcePort, dest, destPort)
		return source, sourcePort, nil
	}

	// if we get here, this means we received a TCP packet
	// but it's not from who we were expecting
	// TODO: this could still be a valid packet, but the
	// host could be behind a NAT/LB?
	// for now return a mismatch error which we can ignore
	return net.IP{}, 0, MismatchError(fmt.Sprintf("received non-matching TCP reply from %s:%d", source.String(), sourcePort))
}

func (c CanceledError) Error() string {
	return string(c)
}

func (m MismatchError) Error() string {
	return string(m)
}
