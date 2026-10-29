// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || (linux && bpf) || darwin

package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"runtime"
	"syscall"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxIPBufferSize = 200

var (
	errTruncated      = errors.New("the packet is truncated")
	errSkippedPayload = errors.New("the packet does not contain relevant DNS response")

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
	udpPayload         *udpWithDNSSupport
	tcpPayload         *tcpWithDNSSupport
	dnsPayload         *layers.DNS
	collectDNSStats    bool
	collectDNSDomains  bool
	recordedQueryTypes map[layers.DNSType]struct{}

	// trailing holds the responses found after the first message of the last
	// TCP segment passed to ParseInto. It is reused across calls.
	trailing []dnsPacketInfo
}

func newDNSParser(layerType gopacket.LayerType, cfg *config.Config) *dnsParser {
	ipv4Payload := &layers.IPv4{}
	ipv6Payload := &layers.IPv6{}
	udpPayload := &udpWithDNSSupport{}
	tcpPayload := &tcpWithDNSSupport{}
	dnsPayload := &layers.DNS{}
	queryTypes := getRecordedQueryTypes(cfg)

	var linkLayer gopacket.DecodingLayer
	if layerType == layers.LayerTypeLoopback {
		linkLayer = &layers.Loopback{}
	} else {
		linkLayer = &layers.Ethernet{}
	}
	stack := []gopacket.DecodingLayer{
		linkLayer,
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

// ParseInto decodes the DNS message at the start of data into pktInfo. For a
// TCP segment, it also returns the responses that follow the first message,
// read from their headers only. They are returned even when the first message
// gives an error, and the slice is reused by the next call.
func (p *dnsParser) ParseInto(data []byte, t *translation, pktInfo *dnsPacketInfo) ([]dnsPacketInfo, error) {
	p.trailing = p.trailing[:0]
	err := p.parseFirstMessageInto(data, t, pktInfo)
	if p.collectDNSStats && p.transport() == layers.LayerTypeTCP {
		p.appendTrailingResponses()
	}
	return p.trailing, err
}

func (p *dnsParser) parseFirstMessageInto(data []byte, t *translation, pktInfo *dnsPacketInfo) error {
	err := p.decoder.DecodeLayers(data, &p.layers)

	// If there is a DNS layer then it would be the last layer. A truncated TCP
	// segment can still start with a complete message, so take the normal path
	// whenever the DNS layer was decoded.
	if err != nil || len(p.layers) == 0 || p.layers[len(p.layers)-1] != layers.LayerTypeDNS {
		// The whole DNS message isn't available or couldn't be decoded: the
		// packet was cut short by the snaplen, it's a TCP segment holding part of
		// a message, or the message is malformed. A response can still be matched
		// to its query by its header.
		if p.collectDNSStats && p.parseResponseHeaderInto(pktInfo) {
			if err != nil {
				// the message was complete but failed to decode, so its answers are lost
				snooperTelemetry.decodingErrors.Inc()
			}
			return nil
		}
		if p.decoder.Truncated {
			return errTruncated
		}
		if err != nil {
			return err
		}
		return errSkippedPayload
	}

	if err := p.parseAnswerInto(p.dnsPayload, t, pktInfo); err != nil {
		return err
	}

	if !p.collectDNSStats {
		return nil
	}

	p.setKey(pktInfo)
	pktInfo.transactionID = p.dnsPayload.ID
	return nil
}

// maxDNSNameLen is the maximum length of a domain name in wire format
const maxDNSNameLen = 255

// questionTypeAndClass returns the type and class of the question that follows
// the header of msg, or false if the question name is malformed or msg ends
// before the question does.
func questionTypeAndClass(msg []byte) (qtype, qclass uint16, ok bool) {
	i := dnsHeaderLen
	for i < len(msg) && i-dnsHeaderLen <= maxDNSNameLen {
		label := int(msg[i])
		switch {
		case label == 0:
			i++
		case label&0xc0 == 0xc0: // a compression pointer ends the name
			i += 2
		case label&0xc0 == 0:
			i += 1 + label
			continue
		default:
			return 0, 0, false
		}
		if i+4 > len(msg) {
			return 0, 0, false
		}
		return binary.BigEndian.Uint16(msg[i:]), binary.BigEndian.Uint16(msg[i+2:]), true
	}
	return 0, 0, false
}

// transport returns the transport layer of the last decoded packet: the last
// decoded layer, or the one before the DNS layer.
func (p *dnsParser) transport() gopacket.LayerType {
	n := len(p.layers)
	if n > 0 && p.layers[n-1] == layers.LayerTypeDNS {
		n--
	}
	if n == 0 {
		return gopacket.LayerTypeZero
	}
	return p.layers[n-1]
}

// parseResponseHeaderInto fills pktInfo from the fixed header of a DNS response
// whose full message couldn't be decoded. The header is enough for DNS stats:
// the response is matched to its query by ID, and the name and type come from
// the query. It returns false if the payload doesn't start with a response header.
func (p *dnsParser) parseResponseHeaderInto(pktInfo *dnsPacketInfo) bool {
	var ok bool
	switch p.transport() {
	case layers.LayerTypeUDP:
		ok = p.setResponseFromHeader(p.udpPayload.UDP.LayerPayload(), pktInfo)
	case layers.LayerTypeTCP:
		payload := p.tcpPayload.TCP.LayerPayload()
		// The message usually starts after its 2-byte length, but the length
		// may have been sent in a segment of its own.
		ok = (len(payload) > 2 && p.setResponseFromHeader(payload[2:], pktInfo)) || p.setResponseFromHeader(payload, pktInfo)
	}
	if !ok {
		return false
	}
	p.setKey(pktInfo)
	pktInfo.partial = true
	return true
}

// appendTrailingResponses records the responses that follow the first message
// of a TCP segment, as happens with pipelined queries. Only their headers are
// read. Messages are delimited by their length fields, so one that isn't a
// response is skipped, and the walk stops at an implausible length or at the
// end of the segment.
func (p *dnsParser) appendTrailingResponses() {
	payload := p.tcpPayload.TCP.LayerPayload()
	if len(payload) < 2 {
		return
	}
	next := 2 + int(binary.BigEndian.Uint16(payload))
	if next-2 < dnsHeaderLen || next > len(payload) {
		return // the first message doesn't end in this segment
	}
	var key Key // the same for every response in the segment
	for rest := payload[next:]; len(rest) > 2; rest = rest[next:] {
		length := int(binary.BigEndian.Uint16(rest))
		if length < dnsHeaderLen {
			return
		}
		response := dnsPacketInfo{partial: true}
		if p.setResponseFromHeader(rest[2:], &response) {
			if len(p.trailing) == 0 {
				p.setKey(&response)
				key = response.key
			}
			response.key = key
			p.trailing = append(p.trailing, response)
		}
		next = 2 + length
		if next > len(rest) {
			return
		}
	}
}

// setResponseFromHeader reads the ID and response code of a DNS response from
// the start of msg, if its question is of a recorded type, as parseAnswerInto
// requires of whole messages. The question must be complete, which also makes
// data from the middle of a message very unlikely to pass. A false positive
// would still need to match the ID of a pending query on the same connection.
func (p *dnsParser) setResponseFromHeader(msg []byte, pktInfo *dnsPacketInfo) bool {
	if !isStandardQueryHeader(msg) {
		return false
	}
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&0x8000 == 0 { // not a response
		return false
	}
	qtype, qclass, ok := questionTypeAndClass(msg)
	if !ok || layers.DNSClass(qclass) != layers.DNSClassIN || !p.isWantedQueryType(layers.DNSType(qtype)) {
		return false
	}
	pktInfo.transactionID = binary.BigEndian.Uint16(msg[0:2])
	pktInfo.queryType = QueryType(qtype)
	pktInfo.rCode = uint8(flags & 0xf)
	if pktInfo.rCode == 0 {
		pktInfo.pktType = successfulResponse
	} else {
		pktInfo.pktType = failedResponse
	}
	return true
}

// setKey fills the connection key of pktInfo from the decoded IP and transport layers
func (p *dnsParser) setKey(pktInfo *dnsPacketInfo) {
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
		return getDefaultRecordedQueryTypes()
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
		return getDefaultRecordedQueryTypes()
	}
	return queryTypes
}

func getDefaultRecordedQueryTypes() map[layers.DNSType]struct{} {
	// recordedRecordTypes defines a map of DNS types that we'll capture by default.
	// add additional types here to change the default.
	defaultRecordedQueryTypes := map[layers.DNSType]struct{}{
		layers.DNSTypeA: {},
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// ipv6 DNS is currently not supported on Windows
		// TODO: Add layers.DNSTypeAAAA for windows once supported
		defaultRecordedQueryTypes[layers.DNSTypeAAAA] = struct{}{}
	}
	return defaultRecordedQueryTypes
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
