package ebpf

import (
	"bytes"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/afpacket"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	bpflib "github.com/iovisor/gobpf/elf"
)

const (
	dnsCacheTTL              = 3 * time.Minute
	dnsCacheExpirationPeriod = 1 * time.Minute
)

type NamePair struct {
	Source, Dest string
}

type ReverseDNS interface {
	Resolve([]ConnectionStats) []NamePair
	Close()
}

var _ ReverseDNS = &SocketFilterSnooper{}

type SocketFilterSnooper struct {
	tpacket      *afpacket.TPacket
	socketFilter *bpflib.SocketFilter
	ipsToNames   *reverseDNSCache
	exit         chan struct{}
}

type translation struct {
	name string
	ips  map[util.Address]struct{}
}

func NewSocketFilterSnooper(filter *bpflib.SocketFilter) (*SocketFilterSnooper, error) {
	tpacket, err := afpacket.NewTPacket()
	if err != nil {
		return nil, fmt.Errorf("error creating raw socket: %s", err)
	}

	err = bpflib.AttachSocketFilter(filter, tpacket.Fd())
	if err != nil {
		return nil, fmt.Errorf("error attaching filter to socket: %s", err)
	}

	snooper := &SocketFilterSnooper{
		tpacket:      tpacket,
		socketFilter: filter,
		exit:         make(chan struct{}),
		ipsToNames:   newReverseDNSCache(dnsCacheTTL, dnsCacheExpirationPeriod),
	}

	go snooper.run()
	return snooper, nil
}

func (s *SocketFilterSnooper) Resolve(connections []ConnectionStats) []NamePair {
	return s.ipsToNames.Get(connections)
}

func (s *SocketFilterSnooper) Close() {
	err := bpflib.DetachSocketFilter(s.socketFilter, s.tpacket.Fd())
	if err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}
	s.exit <- struct{}{}
	s.tpacket.Close()
	s.ipsToNames.Close()
}

func (s *SocketFilterSnooper) run() {
	packets := gopacket.NewPacketSource(s.tpacket, layers.LayerTypeEthernet).Packets()
	for {
		select {
		case packet := <-packets:
			layer := packet.Layer(layers.LayerTypeDNS)
			if layer.LayerType() != layers.LayerTypeDNS {
				continue
			}
			dns, ok := layer.(*layers.DNS)
			if !ok {
				continue
			}

			translation := parseAnswer(dns)
			if translation == nil {
				continue
			}

			s.ipsToNames.Add(translation)
		case <-s.exit:
			return
		}
	}
}

// source: https://github.com/weaveworks/scope/blob/c5ac315b383fdf47c57cebb30bb2b7edd437ec74/probe/endpoint/dns_snooper_linux_amd64.go
func parseAnswer(dns *layers.DNS) *translation {
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
		records       = append(dns.Answers, dns.Additionals...)
		ips           = map[util.Address]struct{}{}
		aliases       = [][]byte{}
	)

	// Traverse all the CNAME records and the get the aliases. There are when the A record is for only one of the aliases.
	// We traverse CNAME records first because there is no guarantee that the A records will be the first ones.
	for _, record := range records {
		if record.Type == layers.DNSTypeCNAME && record.Class == layers.DNSClassIN {
			aliases = append(aliases, record.CNAME)
		}
	}

	// Finally, get the answer
	for _, record := range records {
		if record.Type != layers.DNSTypeA || record.Class != layers.DNSClassIN {
			continue
		}
		if bytes.Equal(domainQueried, record.Name) {
			ips[util.AddressFromNetIP(record.IP)] = struct{}{}
			continue
		}
		for _, alias := range aliases {
			if bytes.Equal(alias, record.Name) {
				ips[util.AddressFromNetIP(record.IP)] = struct{}{}
				break
			}
		}
	}

	return &translation{name: string(domainQueried), ips: ips}
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []ConnectionStats) []NamePair {
	return nil
}

func (nullReverseDNS) Close() {
	return
}

var _ ReverseDNS = nullReverseDNS{}
