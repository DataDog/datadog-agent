// +build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"github.com/DataDog/dd-go/log"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
)

const maxIPBufferSize = 200

var errDNSParsing = errors.New("error parsing DNS payload")

// Gopacket doesn't provide direct support for DNS over TCP, see https://github.com/google/gopacket/issues/236
type tcpWithDNSSupport struct {
	tcp layers.TCP
}

func (m *tcpWithDNSSupport) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	return m.tcp.DecodeFromBytes(data, df)
}

func (m *tcpWithDNSSupport) CanDecode() gopacket.LayerClass { return m.tcp.CanDecode() }

// Determine if a TCP segment contains a full DNS message (i.e. not fragmented)
func (m *tcpWithDNSSupport) hasSelfContainedDNSPayload() bool {
	payload := m.tcp.LayerPayload()
	if len(payload) < 2 {
		return false
	}

	// Assume it's a self-contained DNS message if the Length field
	// matches the length of the TCP segment
	dnsLengthField := binary.BigEndian.Uint16(payload)
	return int(dnsLengthField) == len(payload)-2
}

func (m *tcpWithDNSSupport) NextLayerType() gopacket.LayerType {
	// TODO: deal with TCP fragmentation and out-of-order segments
	if (m.tcp.SrcPort == 53 || m.tcp.DstPort == 53) && m.hasSelfContainedDNSPayload() {
		log.Debug("Found a TCP DNS packet without fragmentation")
		return layers.LayerTypeDNS
	}
	log.Debug("Found a TCP DNS packet WITH fragmentation")
	return m.tcp.NextLayerType()
}

func (m *tcpWithDNSSupport) LayerPayload() []byte {
	payload := m.tcp.LayerPayload()
	if len(payload) > 1 && (m.tcp.SrcPort == 53 || m.tcp.DstPort == 53) {
		// Omit the DNS length field, only included
		// in TCP, in order to reuse the DNS UDP parser
		payload = payload[2:]
	}
	return payload
}


type dnsParser struct {
	decoder *gopacket.DecodingLayerParser
	layers  []gopacket.LayerType
	payload *layers.DNS
}

func newDNSParser() *dnsParser {
	payload := &layers.DNS{}

	stack := []gopacket.DecodingLayer{
		&layers.Ethernet{},
		&layers.IPv4{},
		&layers.IPv6{},
		&layers.UDP{},
		&tcpWithDNSSupport{},
		&layers.DNS{},
		payload,
	}

	return &dnsParser{
		decoder: gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, stack...),
		payload: payload,
	}
}

func (p *dnsParser) ParseInto(data []byte, t *translation) error {
	err := p.decoder.DecodeLayers(data, &p.layers)
	if err != nil || p.decoder.Truncated {
		log.Errorf("error happened in ParseInto %s or decoder truncated", err)
		return nil
	} else {
		log.Debug("NO Error happened in ParseInto")
	}

	for _, layer := range p.layers {
		log.Debugf("Current level: %s", layer)
		if layer == layers.LayerTypeDNS {
			return p.parseAnswerInto(p.payload, t)
		}
	}

	return nil
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswerInto(dns *layers.DNS, t *translation) error {
	// Only consider responses to singleton, A-record questions
	if !dns.QR || dns.ResponseCode != 0 || len(dns.Questions) != 1 {
		return errDNSParsing
	}
	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return errDNSParsing
	}

	var alias []byte
	domainQueried := question.Name
	log.Debugf("domain queried=%s", domainQueried)

	// Retrieve the CNAME record, if available.
	alias = p.extractCNAME(domainQueried, dns.Answers)
	if alias == nil {
		alias = p.extractCNAME(domainQueried, dns.Additionals)
	}

	// Get IPs
	p.extractIPsInto(alias, domainQueried, dns.Answers, t)
	p.extractIPsInto(alias, domainQueried, dns.Additionals, t)
	t.dns = string(domainQueried)

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
