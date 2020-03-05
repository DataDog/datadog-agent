// +build linux_bpf

package ebpf

import (
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"strconv"
)

const maxIPBufferSize = 200

var (
	errTruncated   = errors.New("the packet is truncated")
	skippedPayload = errors.New("the packet does not contain relevant DNS response")
)

type dnsParser struct {
	decoder *gopacket.DecodingLayerParser
	layers  []gopacket.LayerType
	ipv4Payload *layers.IPv4
	ipv6Payload *layers.IPv6
	udpPayload *layers.UDP
	tcpPayload *tcpWithDNSSupport
	payload *layers.DNS
	dnsStats map[string]int
}

func newDNSParser() *dnsParser {
	ipv4Payload := &layers.IPv4{}
	ipv6Payload := &layers.IPv6{}
	udpPayload := &layers.UDP{}
	tcpPayload := &tcpWithDNSSupport{}
	payload := &layers.DNS{}

	stack := []gopacket.DecodingLayer{
		&layers.Ethernet{},
		// &layers.IPv4{},
		// &layers.IPv6{},
		// &layers.UDP{},
		ipv4Payload,
		ipv6Payload,
		udpPayload,
		tcpPayload,
		// &tcpWithDNSSupport{},
		payload,
	}

	return &dnsParser{
		decoder: gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, stack...),
		ipv4Payload: ipv4Payload,
		ipv6Payload: ipv6Payload,
		udpPayload: udpPayload,
		tcpPayload: tcpPayload,
		payload: payload,
		dnsStats: make(map[string]int),
	}
}

func (p *dnsParser) ParseInto(data []byte, t *translation) error {
	err := p.decoder.DecodeLayers(data, &p.layers)

	if p.decoder.Truncated {
		return errTruncated
	}

	if err != nil {
		return err
	}
	var srcIP, dstIP, srcPort, dstPort string

	var connectionType = ""
	var id = ""
	for _, layer := range p.layers {
		// fmt.Println(layer)
		if layer == layers.LayerTypeIPv4 {
			srcIP = p.ipv4Payload.SrcIP.String()
			dstIP = p.ipv4Payload.DstIP.String()
			id = strconv.Itoa(int(p.ipv4Payload.Id))
		}
		if layer == layers.LayerTypeTCP {
			srcPort = p.tcpPayload.SrcPort.String()
			dstPort = p.tcpPayload.DstPort.String()
			connectionType = "TCP"
		}
		if layer == layers.LayerTypeUDP {
			srcPort = p.udpPayload.SrcPort.String()
			dstPort = p.udpPayload.DstPort.String()
			connectionType = "UDP"
		}
		if layer == layers.LayerTypeDNS {
			err := p.parseAnswerInto(p.payload, t)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%s:%s:%s:%s", srcIP, srcPort, dstIP, dstPort)
			fmt.Printf("%s ID:%s Source %s:%s   Destination %s:%s\n", connectionType, id, srcIP, srcPort, dstIP, dstPort)
			p.dnsStats[key] += 1
			// fmt.Println(p.dnsStats)
		}
	}

	return skippedPayload
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswerInto(dns *layers.DNS, t *translation) error {
	// Only consider responses to singleton, A-record questions
	if !dns.QR || dns.ResponseCode != 0 || len(dns.Questions) != 1 {
		return skippedPayload
	}
	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return skippedPayload
	}

	var alias []byte
	domainQueried := question.Name

	// Retrieve the CNAME record, if available.
	alias = p.extractCNAME(domainQueried, dns.Answers)
	if alias == nil {
		alias = p.extractCNAME(domainQueried, dns.Additionals)
	}

	// Get IPs
	p.extractIPsInto(alias, domainQueried, dns.Answers, t)
	p.extractIPsInto(alias, domainQueried, dns.Additionals, t)
	t.dns = string(domainQueried)
	fmt.Println(t.dns)
	fmt.Printf("DNS ID: %d", dns.ID)
	return nil
}

func (*dnsParser) extractCNAME(domainQueried []byte, records []layers.DNSResourceRecord) []byte {
	for _, record := range records {
		if record.Type == layers.DNSTypeCNAME && record.Class == layers.DNSClassIN &&
			bytes.Equal(domainQueried, record.Name) {
			return record.CNAME
		}
	}

	return nil
}

func (*dnsParser) extractIPsInto(alias, domainQueried []byte, records []layers.DNSResourceRecord, t *translation) {
	for _, record := range records {
		if record.Type != layers.DNSTypeA || record.Class != layers.DNSClassIN {
			continue
		}

		if bytes.Equal(domainQueried, record.Name) ||
			(alias != nil && bytes.Equal(alias, record.Name)) {
			t.add(util.AddressFromNetIP(record.IP))
			fmt.Println(util.AddressFromNetIP(record.IP))
		}
	}
}
