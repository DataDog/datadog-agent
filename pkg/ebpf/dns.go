package ebpf

// NamePair contains DNS entries for both Source and Destination
type NamePair struct {
	Source, Dest []string
}

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve([]ConnectionStats) []NamePair
	GetStats() map[string]int64
	Close()
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []ConnectionStats) []NamePair {
	return nil
}

func (nullReverseDNS) GetStats() map[string]int64 {
	return map[string]int64{
		"lookups":  0,
		"resolved": 0,
		"ips":      0,
	}
}

func (nullReverseDNS) Close() {}

var _ ReverseDNS = nullReverseDNS{}
