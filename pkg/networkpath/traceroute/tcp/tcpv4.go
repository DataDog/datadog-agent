package tcp

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
)

type (
	TCPv4 struct {
		Target   net.IP
		srcIP    net.IP // calculated internally
		srcPort  uint16 // calculated internally
		DestPort uint16
		NumPaths uint16
		MinTTL   uint8
		MaxTTL   uint8
		Delay    time.Duration // delay between sending packets (not applicable if we go the serial send/receive route)
		Timeout  time.Duration // full timeout for all packets
	}

	Results struct {
		Source     net.IP
		SourcePort uint16
		Target     net.IP
		DstPort    uint16
		Hops       []*Hop
	}

	Hop struct {
		IP       net.IP
		Port     uint16
		ICMPType layers.ICMPv4TypeCode
	}
)

func (t *TCPv4) TracerouteSequential() (*Results, error) {
	// Get local address for the interface that connects to this
	// host and store in in the probe
	//
	// TODO: do this once for the probe and hang on to the
	// listener until we decide to close the probe
	tcpAddr, err := LocalAddrForTCP4Host(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local addres for target: %w", err)
	}
	t.srcIP = tcpAddr.IP
	t.srcPort = tcpAddr.AddrPort().Port()

	// So far I haven't had success trying to simply create a socket
	// using syscalls directly, but in theory doing so would allow us
	// to avoid creating two listeners since we could see all IP traffic
	// this way
	//
	// Create a raw ICMP listener to catch ICMP responses
	icmpConn, err := net.ListenPacket("ip4:icmp", tcpAddr.IP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create ICMP listener: %w", err)
	}
	defer icmpConn.Close()
	// RawConn is necessary to set the TTL and ID fields
	rawIcmpConn, err := ipv4.NewRawConn(icmpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw ICMP listener: %w", err)
	}

	// Create a raw TCP listener to catch the TCP response from our final
	// hop if we get one
	tcpConn, err := net.ListenPacket("ip4:tcp", tcpAddr.IP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	defer tcpConn.Close()
	log.Debugf("Listening for TCP on: %s\n", tcpAddr.IP.String()+":"+tcpAddr.AddrPort().String())
	// RawConn is necessary to set the TTL and ID fields
	rawTcpConn, err := ipv4.NewRawConn(tcpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw TCP listener: %w", err)
	}

	// hops should be of length # of hops + a hop for the source
	hops := make([]*Hop, 0, t.MaxTTL-t.MinTTL+1)

	hops = append(hops, &Hop{})

	// TODO: better logic around timeout for sequential is needed
	// right now we're just hacking around the existing
	// need to convert uint8 to int for proper converstion to
	// time.Duration
	timeout := t.Timeout / time.Duration(int(t.MaxTTL-t.MinTTL))

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		hop, err := t.sendAndReceive(rawIcmpConn, rawTcpConn, i, timeout)
		// TODO: create an unknown hop here instead
		if err != nil {
			return nil, fmt.Errorf("failed to run traceroute: %w", err)
		}
		hops = append(hops, hop)
	}

	return &Results{
		Source:     t.srcIP,
		SourcePort: t.srcPort,
		Target:     t.Target,
		DstPort:    t.DestPort,
		Hops:       hops,
	}, nil
}

func (t *TCPv4) sendAndReceive(rawIcmpConn *ipv4.RawConn, rawTcpConn *ipv4.RawConn, ttl int, timeout time.Duration) (*Hop, error) {
	flags := byte(0)
	flags |= SYN
	tcpHeader, tcpPacket, err := CreateRawTCPPacket(t.srcIP, t.srcPort, t.Target, t.DestPort, ttl, flags)
	if err != nil {
		log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	log.Debugf("Sending on port: %d\n", t.srcPort)

	err = SendPacket(rawTcpConn, tcpHeader, tcpPacket)
	if err != nil {
		log.Errorf("failed to send TCP SYN: %s", err.Error())
		return nil, err
	}

	// TODO: pass in timeout duration
	hopIP, hopPort, icmpType, err := listenAnyPacket(rawIcmpConn, rawTcpConn, timeout, t.srcIP, t.srcPort, t.Target, t.DestPort)
	if err != nil {
		log.Errorf("failed to listen for packets: %s", err.Error())
		return nil, err
	}
	log.Debugf("Finished loop for TTL %d", ttl)

	return &Hop{
		IP:       hopIP,
		Port:     hopPort,
		ICMPType: icmpType,
	}, nil
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (t *TCPv4) Close() error {
	return nil
}
