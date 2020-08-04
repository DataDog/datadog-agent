// +build linux_bpf

package network

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
)

const maxIPBufferSize = 200

var (
	errTruncated      = errors.New("the packet is truncated")
	errSkippedPayload = errors.New("the packet does not contain relevant DNS response")
)

type dnsParser struct {
	decoder         *gopacket.DecodingLayerParser
	layers          []gopacket.LayerType
	ipv4Payload     *layers.IPv4
	ipv6Payload     *layers.IPv6
	udpPayload      *layers.UDP
	tcpPayload      *tcpWithDNSSupport
	dnsPayload      *layers.DNS
	collectDNSStats bool
}

func newDNSParser(collectDNStats bool) *dnsParser {
	ipv4Payload := &layers.IPv4{}
	ipv6Payload := &layers.IPv6{}
	udpPayload := &layers.UDP{}
	tcpPayload := &tcpWithDNSSupport{}
	dnsPayload := &layers.DNS{}

	stack := []gopacket.DecodingLayer{
		&layers.Ethernet{},
		ipv4Payload,
		ipv6Payload,
		udpPayload,
		tcpPayload,
		dnsPayload,
	}

	return &dnsParser{
		decoder:         gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, stack...),
		ipv4Payload:     ipv4Payload,
		ipv6Payload:     ipv6Payload,
		udpPayload:      udpPayload,
		tcpPayload:      tcpPayload,
		dnsPayload:      dnsPayload,
		collectDNSStats: collectDNStats,
	}
}

func (p *dnsParser) ParseInto(data []byte, t *translation, pktInfo *dnsPacketInfo) error {
	err := p.decoder.DecodeLayers(data, &p.layers)

	if p.decoder.Truncated {
		return errTruncated
	}

	if err != nil {
		return err
	}

	// If there is a DNS layer then it would be the last layer
	if p.layers[len(p.layers)-1] != layers.LayerTypeDNS {
		return errSkippedPayload
	}

	if err := p.parseAnswerInto(p.dnsPayload, t, pktInfo); err != nil {
		return err
	}

	if !p.collectDNSStats {
		return nil
	}

	for _, layer := range p.layers {
		switch layer {
		case layers.LayerTypeIPv4:
			if pktInfo.pktType == Query {
				pktInfo.key.clientIP = util.AddressFromNetIP(p.ipv4Payload.SrcIP)
				pktInfo.key.serverIP = util.AddressFromNetIP(p.ipv4Payload.DstIP)
			} else {
				pktInfo.key.serverIP = util.AddressFromNetIP(p.ipv4Payload.SrcIP)
				pktInfo.key.clientIP = util.AddressFromNetIP(p.ipv4Payload.DstIP)
			}
		case layers.LayerTypeIPv6:
			if pktInfo.pktType == Query {
				pktInfo.key.clientIP = util.AddressFromNetIP(p.ipv6Payload.SrcIP)
				pktInfo.key.serverIP = util.AddressFromNetIP(p.ipv6Payload.DstIP)
			} else {
				pktInfo.key.serverIP = util.AddressFromNetIP(p.ipv6Payload.SrcIP)
				pktInfo.key.clientIP = util.AddressFromNetIP(p.ipv6Payload.DstIP)

			}
		case layers.LayerTypeUDP:
			if pktInfo.pktType == Query {
				pktInfo.key.clientPort = uint16(p.udpPayload.SrcPort)
			} else {
				pktInfo.key.clientPort = uint16(p.udpPayload.DstPort)

			}
			pktInfo.key.protocol = UDP
		case layers.LayerTypeTCP:
			if pktInfo.pktType == Query {
				pktInfo.key.clientPort = uint16(p.tcpPayload.SrcPort)
			} else {
				pktInfo.key.clientPort = uint16(p.tcpPayload.DstPort)
			}
			pktInfo.key.protocol = TCP
		}
	}

	pktInfo.transactionID = p.dnsPayload.ID
	return nil
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswerInto(
	dns *layers.DNS,
	t *translation,
	pktInfo *dnsPacketInfo,
) error {
	// Only consider singleton, A-record questions
	if len(dns.Questions) != 1 {
		return errSkippedPayload
	}

	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return errSkippedPayload
	}

	// Only consider responses
	if !dns.QR {
		pktInfo.pktType = Query
		return nil
	}

	if dns.ResponseCode != 0 {
		pktInfo.pktType = FailedResponse
		return nil
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

	pktInfo.pktType = SuccessfulResponse
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
		}
	}
}
