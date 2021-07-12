package network

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/google/gopacket/layers"
)

// QueryType is the DNS record type
type QueryType layers.DNSType

// DNSType known values.
const (
	DNSTypeA     QueryType = 1   // a host address
	DNSTypeNS    QueryType = 2   // an authoritative name server
	DNSTypeMD    QueryType = 3   // a mail destination (Obsolete - use MX)
	DNSTypeMF    QueryType = 4   // a mail forwarder (Obsolete - use MX)
	DNSTypeCNAME QueryType = 5   // the canonical name for an alias
	DNSTypeSOA   QueryType = 6   // marks the start of a zone of authority
	DNSTypeMB    QueryType = 7   // a mailbox domain name (EXPERIMENTAL)
	DNSTypeMG    QueryType = 8   // a mail group member (EXPERIMENTAL)
	DNSTypeMR    QueryType = 9   // a mail rename domain name (EXPERIMENTAL)
	DNSTypeNULL  QueryType = 10  // a null RR (EXPERIMENTAL)
	DNSTypeWKS   QueryType = 11  // a well known service description
	DNSTypePTR   QueryType = 12  // a domain name pointer
	DNSTypeHINFO QueryType = 13  // host information
	DNSTypeMINFO QueryType = 14  // mailbox or mail list information
	DNSTypeMX    QueryType = 15  // mail exchange
	DNSTypeTXT   QueryType = 16  // text strings
	DNSTypeAAAA  QueryType = 28  // a IPv6 host address [RFC3596]
	DNSTypeSRV   QueryType = 33  // server discovery [RFC2782] [RFC6195]
	DNSTypeOPT   QueryType = 41  // OPT Pseudo-RR [RFC6891]
	DNSTypeURI   QueryType = 256 // URI RR [RFC7553]
)

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve([]ConnectionStats) map[util.Address][]string
	GetDNSStats() map[DNSKey]map[string]map[QueryType]DNSStats
	GetStats() map[string]int64
	Close()
}

// NewNullReverseDNS returns a dummy implementation of ReverseDNS
func NewNullReverseDNS() ReverseDNS {
	return nullReverseDNS{}
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []ConnectionStats) map[util.Address][]string {
	return nil
}

func (nullReverseDNS) GetDNSStats() map[DNSKey]map[string]map[QueryType]DNSStats {
	return nil
}

func (nullReverseDNS) GetStats() map[string]int64 {
	return map[string]int64{
		"lookups":           0,
		"resolved":          0,
		"ips":               0,
		"added":             0,
		"expired":           0,
		"packets_received":  0,
		"packets_processed": 0,
		"packets_dropped":   0,
		"socket_polls":      0,
		"decoding_errors":   0,
		"errors":            0,
	}
}

func (nullReverseDNS) Close() {}

var _ ReverseDNS = nullReverseDNS{}
