/* SPDX-License-Identifier: BSD-2-Clause */

// Package probev4 provides a probe type based on IPv4 and UDP
package probev4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	inet "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/net"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/probes"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/results"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/net/ipv4"
)

// UDPv4 is a probe type based on IPv4 and UDP
type UDPv4 struct {
	Target     net.IP
	SrcPort    uint16
	DstPort    uint16
	UseSrcPort bool
	NumPaths   uint16
	MinTTL     uint8
	MaxTTL     uint8
	Delay      time.Duration
	Timeout    time.Duration
	// TODO implement broken nat detection
	BrokenNAT bool
}

func computeFlowhash(p *ProbeResponseUDPv4) (uint16, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}
	var flowhash uint16
	flowhash += uint16(p.InnerIP().TOS) + uint16(p.InnerIP().Protocol)
	flowhash += binary.BigEndian.Uint16(p.InnerIP().Src.To4()[:2]) + binary.BigEndian.Uint16(p.InnerIP().Src.To4()[2:4])
	flowhash += binary.BigEndian.Uint16(p.InnerIP().Dst.To4()[:2]) + binary.BigEndian.Uint16(p.InnerIP().Dst.To4()[2:4])
	flowhash += uint16(p.InnerUDP().Src) + uint16(p.InnerUDP().Dst)
	return flowhash, nil
}

// Validate checks that the probe is configured correctly and it is safe to
// subsequently run the Traceroute() method
func (d *UDPv4) Validate() error {
	if d.Target.To4() == nil {
		return errors.New("Invalid IPv4 address")
	}
	if d.NumPaths == 0 {
		return errors.New("Number of paths must be a positive integer")
	}
	if d.UseSrcPort {
		if d.SrcPort+d.NumPaths > 0xffff {
			return errors.New("Source port plus number of paths cannot exceed 65535")
		}
	} else {
		if d.DstPort+d.NumPaths > 0xffff {
			return errors.New("Destination port plus number of paths cannot exceed 65535")
		}
	}
	if d.MinTTL == 0 {
		return errors.New("Minimum TTL must be a positive integer")
	}
	if d.MaxTTL < d.MinTTL {
		return errors.New("Invalid maximum TTL, must be greater or equal than minimum TTL")
	}
	if d.Delay < 1 {
		return errors.New("Invalid delay, must be positive")
	}
	return nil
}

// packet generates a probe packet and returns its IPv4 header and payload.
func (d UDPv4) packet(ttl uint8, src, dst net.IP, srcport, dstport uint16) (*ipv4.Header, []byte, error) {
	// forge the payload. The last two bytes will be adjusted to have a
	// predictable checksum for NAT detection
	payload := []byte("NSMNC\x00\x00\x00")
	id := uint16(ttl)
	if d.UseSrcPort {
		id += srcport
	} else {
		id += dstport
	}
	payload[6] = byte((id >> 8) & 0xff)
	payload[7] = byte(id & 0xff)

	iph := ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		Flags:    ipv4.DontFragment,
		TTL:      int(ttl),
		Protocol: int(inet.ProtoUDP),
		Src:      src,
		Dst:      dst,
	}
	// compute checksum, so we can assign its value to the IP ID. This will
	// allow tracking packets even if they are rewritten by a NAT.
	pseudoheader, err := inet.IPv4HeaderToPseudoHeader(&iph, inet.UDPHeaderLen+len(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute IPv4 pseudoheader: %w", err)
	}
	udp := inet.UDP{
		Src:          srcport,
		Dst:          dstport,
		Len:          uint16(inet.UDPHeaderLen + len(payload)),
		Payload:      payload,
		PseudoHeader: pseudoheader,
	}
	udpBytes, err := udp.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal UDP header: %w", err)
	}
	iph.ID = int(udp.Csum)
	iph.TotalLen = ipv4.HeaderLen + len(udpBytes)

	return &iph, udpBytes, nil
}

type pkt struct {
	Header  *ipv4.Header
	Payload []byte
	Port    int
}

// packets returns a channel of packets that will be sent as probes
func (d UDPv4) packets(src, dst net.IP) <-chan pkt {
	numPackets := int(d.NumPaths) * int(d.MaxTTL-d.MinTTL)
	ret := make(chan pkt, numPackets)

	go func() {
		var (
			srcPort, dstPort, basePort uint16
		)
		if d.UseSrcPort {
			basePort = d.SrcPort
		} else {
			basePort = d.DstPort
		}
		for ttl := d.MinTTL; ttl <= d.MaxTTL; ttl++ {
			for port := basePort; port < basePort+d.NumPaths; port++ {
				if d.UseSrcPort {
					srcPort = port
					dstPort = d.DstPort
				} else {
					srcPort = d.SrcPort
					dstPort = port
				}
				iph, payload, err := d.packet(ttl, src, dst, srcPort, dstPort)
				if err != nil {
					log.Errorf("Warning: cannot generate packet for ttl=%d srcport=%d dstport=%d: %v",
						ttl, srcPort, dstPort, err,
					)
				} else {
					ret <- pkt{Header: iph, Payload: payload, Port: int(dstPort)}
				}
			}
		}
		close(ret)
	}()
	return ret
}

// SendReceive sends all the packets to the target address, respecting the configured
// inter-packet delay
func (d UDPv4) SendReceive() ([]probes.Probe, []probes.ProbeResponse, error) {
	localAddr, err := inet.GetLocalAddr("udp4", d.Target)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get local address for target %s with network type 'udp4': %w", d.Target, err)
	}
	localUDPAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return nil, nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localUDPAddr, localAddr)
	}
	// listen for IPv4/ICMP traffic back
	conn, err := net.ListenPacket("ip4:icmp", localUDPAddr.IP.String())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ICMPv4 packet listener: %w", err)
	}
	defer conn.Close()
	// RawConn is necessary to set the TTL and ID fields
	rconn, err := ipv4.NewRawConn(conn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new RawConn: %w", err)
	}

	numPackets := int(d.NumPaths) * int(d.MaxTTL-d.MinTTL)

	// spawn the listener
	recvErrors := make(chan error)
	recvChan := make(chan []probes.ProbeResponse, 1)
	go func(errch chan error, rc chan []probes.ProbeResponse) {
		howLong := d.Delay*time.Duration(numPackets) + d.Timeout
		received, err := d.ListenFor(rconn, howLong)
		errch <- err
		// TODO pass the rp chan to ListenFor and let it feed packets there
		rc <- received
	}(recvErrors, recvChan)

	// send the packets
	sent := make([]probes.Probe, 0, numPackets)
	for p := range d.packets(localUDPAddr.IP, d.Target) {
		if err := rconn.WriteTo(p.Header, p.Payload, nil); err != nil {
			return nil, nil, fmt.Errorf("failed to send IPv4 packet: %w", err)
		}
		// get the current time as soon as possible after sending the packet.
		// TODO use kernel timestamping (and equivalent on other platforms) for
		//      more accurate time measurement.
		ts := time.Now()
		data, err := p.Header.Marshal()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal IPv4 header: %w", err)
		}
		data = append(data, p.Payload...)
		sent = append(sent, &ProbeUDPv4{Data: data, LocalAddr: localUDPAddr.IP, Timestamp: ts})
		time.Sleep(d.Delay)
	}

	if err = <-recvErrors; err != nil {
		return nil, nil, err
	}
	received := <-recvChan
	return sent, received, nil
}

// ListenFor waits for ICMP packets until the timeout expires
func (d UDPv4) ListenFor(rconn *ipv4.RawConn, howLong time.Duration) ([]probes.ProbeResponse, error) {
	packets := make([]probes.ProbeResponse, 0)
	deadline := time.Now().Add(howLong)
	for {
		if time.Until(deadline) <= 0 {
			break
		}
		// TODO tune data size
		data := make([]byte, 1024)
		now := time.Now()
		err := rconn.SetReadDeadline(now.Add(time.Millisecond * 100))
		if err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}
		hdr, payload, _, err := rconn.ReadFrom(data)
		receivedAt := time.Now()
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok {
				if nerr.Timeout() {
					continue
				}
				return nil, err
			}
		}
		packets = append(packets, &ProbeResponseUDPv4{
			Header:    hdr,
			Payload:   payload,
			Timestamp: receivedAt,
		})
	}
	return packets, nil
}

// Match compares the sent and received packets and finds the matching ones. It
// returns a Results structure.
func (d UDPv4) Match(sent []probes.Probe, received []probes.ProbeResponse) results.Results {
	res := results.Results{
		Flows: make(map[uint16][]results.Probe),
	}

	for _, sp := range sent {
		spu := sp.(*ProbeUDPv4)
		if err := spu.Validate(); err != nil {
			log.Debugf("Skipping invalid probe: %v", err)
			continue
		}
		sentIP := spu.IP()
		sentUDP := spu.UDP()
		probe := results.Probe{
			Sent: results.Packet{
				Timestamp: results.UnixUsec(spu.Timestamp),
				IP: results.IP{
					SrcIP: spu.LocalAddr,
					DstIP: sentIP.Dst,
					TTL:   uint8(sentIP.TTL),
				},
				UDP: &results.UDP{
					SrcPort: uint16(sentUDP.Src),
					DstPort: uint16(sentUDP.Dst),
				},
			},
		}
		var flowID uint16
		if d.UseSrcPort {
			flowID = uint16(sentUDP.Src)
		} else {
			flowID = uint16(sentUDP.Dst)
		}
		for _, rp := range received {
			rpu := rp.(*ProbeResponseUDPv4)
			if err := rpu.Validate(); err != nil {
				continue
			}
			if !rpu.Matches(spu) {
				continue
			}

			// the two packets belong to the same flow. If the checksums
			// differ there's a NAT
			NATID := rpu.InnerUDP().Csum - sentUDP.Csum
			// TODO this works when the source port is fixed. Allow for variable
			//      source port too
			flowhash, err := computeFlowhash(rpu)
			if err != nil {
				log.Debugf("Failed to compute flowhash: %s", err.Error())
				continue
			}
			description := "Unknown"
			isPortUnreachable := false
			if rpu.ICMP().Type == inet.ICMPDestUnreachable && rpu.ICMP().Code == 3 {
				isPortUnreachable = true
				description = "Destination port unreachable"
			} else if rpu.ICMP().Type == inet.ICMPTimeExceeded && rpu.ICMP().Code == 0 {
				description = "TTL expired in transit"
			}
			// This is our packet, let's fill the probe data up
			probe.Flowhash = flowhash
			probe.IsLast = rpu.Header.Src.To4().Equal(d.Target.To4()) || isPortUnreachable
			probe.Name = rpu.Header.Src.String() // TODO compute this field
			probe.RttUsec = uint64(rpu.Timestamp.Sub(spu.Timestamp)) / 1000
			probe.NATID = NATID
			probe.ZeroTTLForwardingBug = (rpu.InnerIP().TTL == 0)
			probe.Received = &results.Packet{
				Timestamp: results.UnixUsec(rpu.Timestamp),
				ICMP: &results.ICMP{
					Type:        uint8(rpu.ICMP().Type),
					Code:        uint8(rpu.ICMP().Code),
					Description: description,
				},
				IP: results.IP{
					SrcIP: rpu.Header.Src,
					DstIP: spu.LocalAddr,
					TTL:   uint8(rpu.InnerIP().TTL),
				},
				UDP: &results.UDP{
					SrcPort: uint16(rpu.InnerUDP().Src),
					DstPort: uint16(rpu.InnerUDP().Dst),
				},
			}
			// break, since this is a response to the sent probe
			break
		}
		res.Flows[flowID] = append(res.Flows[flowID], probe)
	}
	return res
}

// Traceroute sends the probes and returns a Results structure or an error
func (d UDPv4) Traceroute() (*results.Results, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	sent, received, err := d.SendReceive()
	if err != nil {
		return nil, err
	}
	results := d.Match(sent, received)

	return &results, nil
}
