package network

import "github.com/DataDog/datadog-agent/pkg/process/util"

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve([]ConnectionStats) map[util.Address][]string
	GetStats() map[string]int64
	Close()
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []ConnectionStats) map[util.Address][]string {
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
	}
}

func (nullReverseDNS) Close() {}

var _ ReverseDNS = nullReverseDNS{}
