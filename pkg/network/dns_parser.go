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

	if p.decoder.Truncated {
		return errTruncated
	}

	if err != nil {
		return err
	}

	for _, layer := range p.layers {
		if layer == layers.LayerTypeDNS {
			return p.parseAnswerInto(p.payload, t)
		}
	}

	return errSkippedPayload
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswerInto(dns *layers.DNS, t *translation) error {
	// Only consider responses to singleton, A-record questions
	if !dns.QR || dns.ResponseCode != 0 || len(dns.Questions) != 1 {
		return errSkippedPayload
	}
	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return errSkippedPayload
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
