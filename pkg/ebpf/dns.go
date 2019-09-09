package ebpf

// NamePair contains DNS entries for both Source and Destination
type NamePair struct {
	Source, Dest string
}

// ReverseDNS translates IPs to names
type ReverseDNS interface {
	Resolve([]ConnectionStats) []NamePair
	Close()
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []ConnectionStats) []NamePair {
	return nil
}

func (nullReverseDNS) Close() {
	return
}

var _ ReverseDNS = nullReverseDNS{}
