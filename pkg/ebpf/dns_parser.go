// +build linux_bpf

package ebpf

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
)

const maxIPBufferSize = 200

var errNoDNSLayer = errors.New("parsed layers do not contain a DNS layer")
var errParsing = errors.New("error parsing packet")
var errUnhandledDNSResponse = errors.New("unsupported DNS response")
var errTruncated = errors.New("the packet is truncated")

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
		payload,
	}

	return &dnsParser{
		decoder: gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, stack...),
		payload: payload,
	}
}

func (p *dnsParser) ParseInto(data []byte, t *translation) error {
	err := p.decoder.DecodeLayers(data, &p.layers)
	// Parsing errors can be of different types. For example, if a DNS
	// payload gets fragmented into two TCP segments, parsing of the two segments
	// will result in two different errors (errDecodeRecordLength and
	// errDNSNameOffsetNegative respectively). Instead of propagating those
	// fine-grained errors upstream, we return a generic error to represent all
	// parsing errors.
	if err != nil {
		return errParsing
	}

	if p.decoder.Truncated {
		return errTruncated
	}

	for _, layer := range p.layers {
		if layer == layers.LayerTypeDNS {
			return p.parseAnswerInto(p.payload, t)
		}
	}

	return errNoDNSLayer
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswerInto(dns *layers.DNS, t *translation) error {
	// Only consider responses to singleton, A-record questions
	if !dns.QR || dns.ResponseCode != 0 || len(dns.Questions) != 1 {
		return errUnhandledDNSResponse
	}
	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return errUnhandledDNSResponse
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
