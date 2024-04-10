// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package dns

import (
	"github.com/gopacket/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

var si = intern.NewStringInterner()

// Hostname represents a DNS hostname (aka domain name)
type Hostname = *intern.StringValue

// ToString converts a dns.Hostname to a string
func ToString(h Hostname) string {
	return h.Get()
}

// HostnameFromBytes converts a byte slice representing a hostname to a dns.Hostname
func HostnameFromBytes(b []byte) Hostname {
	return si.Get(b)
}

// ToHostname converts from a string to a dns.Hostname
func ToHostname(s string) Hostname {
	return si.GetString(s)
}

// QueryType is the DNS record type
type QueryType layers.DNSType

// DNSType known values.
const (
	TypeA     QueryType = 1   // a host address
	TypeNS    QueryType = 2   // an authoritative name server
	TypeMD    QueryType = 3   // a mail destination (Obsolete - use MX)
	TypeMF    QueryType = 4   // a mail forwarder (Obsolete - use MX)
	TypeCNAME QueryType = 5   // the canonical name for an alias
	TypeSOA   QueryType = 6   // marks the start of a zone of authority
	TypeMB    QueryType = 7   // a mailbox domain name (EXPERIMENTAL)
	TypeMG    QueryType = 8   // a mail group member (EXPERIMENTAL)
	TypeMR    QueryType = 9   // a mail rename domain name (EXPERIMENTAL)
	TypeNULL  QueryType = 10  // a null RR (EXPERIMENTAL)
	TypeWKS   QueryType = 11  // a well known service description
	TypePTR   QueryType = 12  // a domain name pointer
	TypeHINFO QueryType = 13  // host information
	TypeMINFO QueryType = 14  // mailbox or mail list information
	TypeMX    QueryType = 15  // mail exchange
	TypeTXT   QueryType = 16  // text strings
	TypeAAAA  QueryType = 28  // a IPv6 host address [RFC3596]
	TypeSRV   QueryType = 33  // server discovery [RFC2782] [RFC6195]
	TypeOPT   QueryType = 41  // OPT Pseudo-RR [RFC6891]
	TypeURI   QueryType = 256 // URI RR [RFC7553]
)

// StatsByKeyByNameByType provides a type name for the map of
// DNS stats based on the host key->the lookup name->querytype
type StatsByKeyByNameByType map[Key]map[Hostname]map[QueryType]Stats

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve(map[util.Address]struct{}) map[util.Address][]Hostname
	GetDNSStats() StatsByKeyByNameByType

	// WaitForDomain is used in tests to ensure a domain has been
	// seen by the ReverseDNS.
	WaitForDomain(domain string) error

	Start() error
	Close()
}

// Key is an identifier for a set of DNS connections
type Key struct {
	ServerIP   util.Address
	ClientIP   util.Address
	ClientPort uint16
	// ConnectionType will be either TCP or UDP
	Protocol uint8
}

// Stats holds statistics corresponding to a particular domain
type Stats struct {
	Timeouts          uint32
	SuccessLatencySum uint64
	FailureLatencySum uint64
	CountByRcode      map[uint32]uint32
}
