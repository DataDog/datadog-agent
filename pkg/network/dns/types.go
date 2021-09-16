package dns

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket/layers"
	"go4.org/intern"
)

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
type StatsByKeyByNameByType map[Key]map[*intern.Value]map[QueryType]Stats

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve([]util.Address) map[util.Address][]string
	GetDNSStats() StatsByKeyByNameByType
	GetStats() map[string]int64
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
