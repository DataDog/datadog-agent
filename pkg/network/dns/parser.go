// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package dns

import (
	"bytes"
	"syscall"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxIPBufferSize = 200

var (
	errTruncated      = errors.New("the packet is truncated")
	errSkippedPayload = errors.New("the packet does not contain relevant DNS response")

	// recordedRecordTypes defines a map of DNS types that we'll capture by default.
	// add additional types here to change the default.
	defaultRecordedQueryTypes = map[layers.DNSType]struct{}{
		layers.DNSTypeA: {},
	}

	// map for translating config strings back to the typed value
	queryTypeStrings = map[string]layers.DNSType{
		layers.DNSTypeA.String():     layers.DNSTypeA,
		layers.DNSTypeNS.String():    layers.DNSTypeNS,
		layers.DNSTypeMD.String():    layers.DNSTypeMD,
		layers.DNSTypeMF.String():    layers.DNSTypeMF,
		layers.DNSTypeCNAME.String(): layers.DNSTypeCNAME,
		layers.DNSTypeSOA.String():   layers.DNSTypeSOA,
		layers.DNSTypeMB.String():    layers.DNSTypeMB,
		layers.DNSTypeMG.String():    layers.DNSTypeMG,
		layers.DNSTypeMR.String():    layers.DNSTypeMR,
		layers.DNSTypeNULL.String():  layers.DNSTypeNULL,
		layers.DNSTypeWKS.String():   layers.DNSTypeWKS,
		layers.DNSTypePTR.String():   layers.DNSTypePTR,
		layers.DNSTypeHINFO.String(): layers.DNSTypeHINFO,
		layers.DNSTypeMINFO.String(): layers.DNSTypeMINFO,
		layers.DNSTypeMX.String():    layers.DNSTypeMX,
		layers.DNSTypeTXT.String():   layers.DNSTypeTXT,
		layers.DNSTypeAAAA.String():  layers.DNSTypeAAAA,
		layers.DNSTypeSRV.String():   layers.DNSTypeSRV,
		layers.DNSTypeOPT.String():   layers.DNSTypeOPT,
		layers.DNSTypeURI.String():   layers.DNSTypeURI,
	}
)

type dnsParser struct {
	decoder            *gopacket.DecodingLayerParser
	layers             []gopacket.LayerType
	ipv4Payload        *layers.IPv4
	ipv6Payload        *layers.IPv6
	udpPayload         *layers.UDP
	tcpPayload         *tcpWithDNSSupport
	dnsPayload         *layers.DNS
	collectDNSStats    bool
	collectDNSDomains  bool
	recordedQueryTypes map[layers.DNSType]struct{}
}

func newDNSParser(layerType gopacket.LayerType, cfg *config.Config) *dnsParser {
	ipv4Payload := &layers.IPv4{}
	ipv6Payload := &layers.IPv6{}
	udpPayload := &layers.UDP{}
	tcpPayload := &tcpWithDNSSupport{}
	dnsPayload := &layers.DNS{}
	queryTypes := getRecordedQueryTypes(cfg)

	stack := []gopacket.DecodingLayer{
		&layers.Ethernet{},
		ipv4Payload,
		ipv6Payload,
		udpPayload,
		tcpPayload,
		dnsPayload,
	}

	qtypelist := make([]string, len(queryTypes))
	i := 0
	for k := range queryTypes {
		qtypelist[i] = k.String()
		i++
	}
	log.Infof("Recording dns query types: %v", qtypelist)
	return &dnsParser{
		decoder:            gopacket.NewDecodingLayerParser(layerType, stack...),
		ipv4Payload:        ipv4Payload,
		ipv6Payload:        ipv6Payload,
		udpPayload:         udpPayload,
		tcpPayload:         tcpPayload,
		dnsPayload:         dnsPayload,
		collectDNSStats:    cfg.CollectDNSStats,
		collectDNSDomains:  cfg.CollectDNSDomains,
		recordedQueryTypes: queryTypes,
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
	if question.Class != layers.DNSClassIN || !p.isWantedQueryType(question.Type) {
		return errSkippedPayload
	}

	// Only consider responses
	if !dns.QR {
		pktInfo.pktType = query
		pktInfo.queryType = QueryType(question.Type)
		if p.collectDNSDomains {
			pktInfo.question = ToHostname(string(question.Name))
		} else {
			pktInfo.question = ToHostname("")
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
	inplaceASCIILower(question.Name)
	t.dns = HostnameFromBytes(question.Name)

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

func (p *dnsParser) isWantedQueryType(checktype layers.DNSType) bool {
	_, ok := p.recordedQueryTypes[checktype]
	return ok
}

func getRecordedQueryTypes(cfg *config.Config) map[layers.DNSType]struct{} {
	if len(cfg.RecordedQueryTypes) <= 0 {
		return defaultRecordedQueryTypes
	}
	queryTypes := make(map[layers.DNSType]struct{})
	//
	// check to see if we're recording more/different than the default
	// query types
	if len(cfg.RecordedQueryTypes) > 0 {

		for _, t := range cfg.RecordedQueryTypes {
			if qt, ok := queryTypeStrings[t]; ok {
				queryTypes[qt] = struct{}{}
			} else {
				log.Warnf("Unknown query type %v, skipping", qt)
			}
		}
	}
	if len(queryTypes) <= 0 {
		log.Warnf("No known query types provided in config, reverting to default")
		return defaultRecordedQueryTypes
	}
	return queryTypes
}

// inplaceASCIILower is an optimized, replace inplace version of bytes.ToLower
// for byte slices knowing they only contain ASCII characters.
func inplaceASCIILower(s []byte) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		s[i] = c
	}
}
