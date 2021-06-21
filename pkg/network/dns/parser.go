//+build windows linux_bpf

package dns

import (
	"bytes"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
)

const maxIPBufferSize = 200

var (
	errTruncated      = errors.New("the packet is truncated")
	errSkippedPayload = errors.New("the packet does not contain relevant DNS response")

	// recordedRecordTypes defines a map of DNS types that we'd like to capture.
	// add additional types here to add DNSQueryTypes that will be recorded
	recordedQueryTypes = map[layers.DNSType]struct{}{
		layers.DNSTypeA:    {},
		layers.DNSTypeAAAA: {}}
)

type dnsParser struct {
	decoder           *gopacket.DecodingLayerParser
	layers            []gopacket.LayerType
	ipv4Payload       *layers.IPv4
	ipv6Payload       *layers.IPv6
	udpPayload        *layers.UDP
	tcpPayload        *tcpWithDNSSupport
	dnsPayload        *layers.DNS
	collectDNSStats   bool
	collectDNSDomains bool
}

func newDNSParser(layerType gopacket.LayerType, collectDNSStats bool, collectDNSDomains bool) *dnsParser {
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
		decoder:           gopacket.NewDecodingLayerParser(layerType, stack...),
		ipv4Payload:       ipv4Payload,
		ipv6Payload:       ipv6Payload,
		udpPayload:        udpPayload,
		tcpPayload:        tcpPayload,
		dnsPayload:        dnsPayload,
		collectDNSStats:   collectDNSStats,
		collectDNSDomains: collectDNSDomains,
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
			if pktInfo.pktType == query {
				pktInfo.key.ClientIP = util.AddressFromNetIP(p.ipv4Payload.SrcIP)
				pktInfo.key.ServerIP = util.AddressFromNetIP(p.ipv4Payload.DstIP)
			} else {
				pktInfo.key.ServerIP = util.AddressFromNetIP(p.ipv4Payload.SrcIP)
				pktInfo.key.ClientIP = util.AddressFromNetIP(p.ipv4Payload.DstIP)
			}
		case layers.LayerTypeIPv6:
			if pktInfo.pktType == query {
				pktInfo.key.ClientIP = util.AddressFromNetIP(p.ipv6Payload.SrcIP)
				pktInfo.key.ServerIP = util.AddressFromNetIP(p.ipv6Payload.DstIP)
			} else {
				pktInfo.key.ServerIP = util.AddressFromNetIP(p.ipv6Payload.SrcIP)
				pktInfo.key.ClientIP = util.AddressFromNetIP(p.ipv6Payload.DstIP)

			}
		case layers.LayerTypeUDP:
			if pktInfo.pktType == query {
				pktInfo.key.ClientPort = uint16(p.udpPayload.SrcPort)
			} else {
				pktInfo.key.ClientPort = uint16(p.udpPayload.DstPort)

			}
			pktInfo.key.Protocol = syscall.IPPROTO_UDP
		case layers.LayerTypeTCP:
			if pktInfo.pktType == query {
				pktInfo.key.ClientPort = uint16(p.tcpPayload.SrcPort)
			} else {
				pktInfo.key.ClientPort = uint16(p.tcpPayload.DstPort)
			}
			pktInfo.key.Protocol = syscall.IPPROTO_TCP
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
	if question.Class != layers.DNSClassIN || !isWantedQueryType(question.Type) {
		return errSkippedPayload
	}

	// Only consider responses
	if !dns.QR {
		pktInfo.pktType = query
		pktInfo.queryType = QueryType(question.Type)
		if p.collectDNSDomains {
			pktInfo.question = string(question.Name)
		}
		return nil
	}

	pktInfo.rCode = uint8(dns.ResponseCode)
	if dns.ResponseCode != 0 {
		pktInfo.pktType = failedResponse
		return nil
	}

	pktInfo.queryType = QueryType(question.Type)
	alias := p.extractCNAME(question.Name, dns.Answers)
	p.extractIPsInto(alias, dns.Answers, t)
	t.dns = string(bytes.ToLower(question.Name))

	pktInfo.pktType = successfulResponse
	return nil
}

func (*dnsParser) extractCNAME(domainQueried []byte, records []layers.DNSResourceRecord) []byte {
	alias := domainQueried
	for _, record := range records {
		if record.Class != layers.DNSClassIN {
			continue
		}
		if record.Type == layers.DNSTypeCNAME && bytes.Equal(alias, record.Name) {
			alias = record.CNAME
		}
	}
	return alias
}

func (*dnsParser) extractIPsInto(alias []byte, records []layers.DNSResourceRecord, t *translation) {
	for _, record := range records {
		if record.Class != layers.DNSClassIN {
			continue
		}
		if len(record.IP) == 0 {
			continue
		}
		if bytes.Equal(alias, record.Name) {
			t.add(util.AddressFromNetIP(record.IP), time.Duration(record.TTL)*time.Second)
		}
	}
}

func isWantedQueryType(checktype layers.DNSType) bool {
	_, ok := recordedQueryTypes[checktype]
	return ok
}
