// +build linux_bpf

package ebpf

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const maxIPBufferSize = 200

type dnsParser struct {
	decoder *gopacket.DecodingLayerParser
	layers  []gopacket.LayerType
	payload *layers.DNS

	// Cached translation object to reduce allocations
	cached *translation
}

func newDNSParser() *dnsParser {
	payload := &layers.DNS{}

	stack := []gopacket.DecodingLayer{
		&layers.Ethernet{},
		&layers.IPv4{},
		&layers.IPv6{},
		&layers.UDP{},
		payload,
	}

	return &dnsParser{
		decoder: gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, stack...),
		payload: payload,
		cached:  new(translation),
	}
}

func (p *dnsParser) Parse(data []byte) *translation {
	err := p.decoder.DecodeLayers(data, &p.layers)
	if err != nil || p.decoder.Truncated {
		return nil
	}

	for _, layer := range p.layers {
		if layer == layers.LayerTypeDNS {
			return p.parseAnswer(p.payload)
		}
	}

	return nil
}

// source: https://github.com/weaveworks/scope
func (p *dnsParser) parseAnswer(dns *layers.DNS) *translation {
	// Only consider responses to singleton, A-record questions
	if !dns.QR || dns.ResponseCode != 0 || len(dns.Questions) != 1 {
		return nil
	}
	question := dns.Questions[0]
	if question.Type != layers.DNSTypeA || question.Class != layers.DNSClassIN {
		return nil
	}

	var (
		domainQueried = question.Name
		translation   = p.getCachedTranslation(domainQueried)
		alias         []byte
	)

	// Retrieve the CNAME record, if available.
	alias = p.extractCNAME(domainQueried, dns.Answers)
	if alias == nil {
		alias = p.extractCNAME(domainQueried, dns.Additionals)
	}

	// Get IPs
	p.extractIPs(translation, alias, domainQueried, dns.Answers)
	p.extractIPs(translation, alias, domainQueried, dns.Additionals)

	return translation
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

func (*dnsParser) extractIPs(t *translation, alias, domainQueried []byte, records []layers.DNSResourceRecord) {
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

func (p *dnsParser) getCachedTranslation(dns []byte) *translation {
	t := p.cached
	t.dns = string(dns)

	// Recycle buffer if necessary
	if t.ips == nil || len(t.ips) > maxIPBufferSize {
		t.ips = make([]util.Address, 30)
	}
	t.ips = t.ips[:0]

	return t
}
