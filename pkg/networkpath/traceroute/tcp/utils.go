package tcp

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

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

func LocalAddrForTCP4Host(destIP net.IP, destPort uint16) (*net.TCPAddr, error) {
	// for macOS support we'd need to change this port to something like 53 as per Dublin Traceroute
	// this is a quick way to get the local address for connecting to the host
	conn, err := net.Dial("tcp4", net.JoinHostPort(destIP.String(), strconv.Itoa(int(destPort))))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr()

	localTCPAddr, ok := localAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localTCPAddr, localAddr)
	}

	return localTCPAddr, nil
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

func ParseTCPPacket(pkt gopacket.Packet) (net.IP, uint16, net.IP, uint16, error) {
	var src net.IP
	var srcPort uint16
	var dst net.IP
	var dstPort uint16

	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		//fmt.Println("This is an IPv4 packet!")
		ip, ok := ipLayer.(*layers.IPv4)
		if !ok {
			return net.IP{}, 0, net.IP{}, 0, fmt.Errorf("failed to assert IPv4 layer type")
		}
		//fmt.Printf("IPv4 layer: %+v\n", ip)

		src = ip.SrcIP
		dst = ip.DstIP
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		//fmt.Println("This is a TCP packet!")
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return net.IP{}, 0, net.IP{}, 0, fmt.Errorf("failed to assert TCP layer type")
		}
		//fmt.Printf("TCP layer: %+v\n", tcp)
		srcPort = uint16(tcp.SrcPort)
		dstPort = uint16(tcp.DstPort)
	}

	return src, srcPort, dst, dstPort, nil
}

// CreateRawTCPPacket creates a TCP packet with the specified parameters
func CreateRawTCPPacket(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, ttl int, flags byte) (*ipv4.Header, []byte, error) {
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
	binary.BigEndian.PutUint32(tcpPacket[4:8], 0)          // sequence number
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

func listenAnyPacket(icmpConn *ipv4.RawConn, tcpConn *ipv4.RawConn, timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (net.IP, uint16, layers.ICMPv4TypeCode, error) {
	var err1 error
	var err2 error
	var wg sync.WaitGroup
	var ip net.IP
	var icmpCode layers.ICMPv4TypeCode
	var port uint16
	wg.Add(2)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		defer wg.Done()
		defer cancel()
		ip, port, _, err1 = handlePackets(ctx, tcpConn, "tcp", localIP, localPort, remoteIP, remotePort)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		ip, _, icmpCode, err2 = handlePackets(ctx, icmpConn, "icmp", localIP, localPort, remoteIP, remotePort)
	}()
	wg.Wait()

	if err1 != nil && err2 != nil {
		_, ok1 := err1.(CanceledError)
		_, ok2 := err2.(CanceledError)
		if ok1 && ok2 {
			// TODO: better handling of listener timeout case
			// this signifies an unknown hop
			//
			// For now, return nil error with empty hop data
			return net.IP{}, 0, 0, nil
		} else {
			return net.IP{}, 0, 0, multierr.Append(err1, err2)
		}
	}

	return ip, port, icmpCode, nil
}

// handlePackets in its current implementation should listen for the first matching
// packet on the connection and then return. If no packet is received within the
// timeout, it should return a timeout exceeded error
func handlePackets(ctx context.Context, conn *ipv4.RawConn, listener string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (net.IP, uint16, layers.ICMPv4TypeCode, error) {
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
			return ip, 0, icmpCode, nil
		}
		if listener == "tcp" {
			ip, port, err = parseTCP(header, packet, localIP, localPort, remoteIP, remotePort)
			if err != nil {
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

	src, _, icmpType, err := ParseICMPPacket(packet)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to parse ICMP packet: %w", err)
	}

	if icmpType == layers.ICMPv4TypeDestinationUnreachable || icmpType == layers.ICMPv4TypeTimeExceeded {
		fmt.Printf("Received ICMP reply: %s from %s\n", icmpType.String(), src.String())
	} else {
		fmt.Printf("Received other ICMP reply: %s from %s\n", icmpType.String(), src.String())
	}

	return src, icmpType, nil
}

func parseTCP(header *ipv4.Header, payload []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (net.IP, uint16, error) {
	packetBytes, err := MarshalPacket(header, payload)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to marshal packet: %w", err)
	}

	packet := ReadRawPacket(packetBytes)
	source, sourcePort, dest, destPort, err := ParseTCPPacket(packet)
	if err != nil {
		return net.IP{}, 0, fmt.Errorf("failed to parse TCP packet: %w", err)
	}

	if source.Equal(remoteIP) && sourcePort == remotePort && dest.Equal(localIP) && destPort == localPort {
		fmt.Printf("Received TCP Reply from: %s:%d\n", source.String(), sourcePort)
	}

	return net.IP{}, 0, fmt.Errorf("")
}

func (c CanceledError) Error() string {
	return string(c)
}

func (m MismatchError) Error() string {
	return string(m)
}
