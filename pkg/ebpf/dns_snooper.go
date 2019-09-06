package ebpf

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
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
	packetBufferSize         = 100
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
	source       *gopacket.PacketSource
	socketFilter *bpflib.SocketFilter
	ipsToNames   *reverseDNSCache
	exit         chan struct{}
	wg           sync.WaitGroup
}

type translation struct {
	name string
	ips  map[util.Address]struct{}
}

func NewSocketFilterSnooper(filter *bpflib.SocketFilter) (*SocketFilterSnooper, error) {
	tpacket, err := afpacket.NewTPacket(afpacket.OptPollTimeout(1 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("error creating raw socket: %s", err)
	}

	packetSrc := gopacket.NewPacketSource(tpacket, layers.LayerTypeEthernet)

	err = bpflib.AttachSocketFilter(filter, tpacket.Fd())
	if err != nil {
		return nil, fmt.Errorf("error attaching filter to socket: %s", err)
	}

	snooper := &SocketFilterSnooper{
		tpacket:      tpacket,
		source:       packetSrc,
		socketFilter: filter,
		exit:         make(chan struct{}),
		ipsToNames:   newReverseDNSCache(dnsCacheTTL, dnsCacheExpirationPeriod),
	}

	// Start consuming packets
	packetChan := snooper.createPacketStream(1000)
	snooper.wg.Add(1)
	go func() {
		snooper.run(packetChan)
		snooper.wg.Done()
	}()

	return snooper, nil
}

func (s *SocketFilterSnooper) Resolve(connections []ConnectionStats) []NamePair {
	return s.ipsToNames.Get(connections)
}

func (s *SocketFilterSnooper) Close() {
	s.exit <- struct{}{}
	close(s.exit)
	s.wg.Wait()
	err := bpflib.DetachSocketFilter(s.socketFilter, s.tpacket.Fd())
	if err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}
	s.tpacket.Close()
	s.ipsToNames.Close()
}

func (s *SocketFilterSnooper) run(packets <-chan gopacket.Packet) {
	for packet := range packets {
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
	}
}

func (s *SocketFilterSnooper) createPacketStream(chanSize int) <-chan gopacket.Packet {
	packetChan := make(chan gopacket.Packet, packetBufferSize)
	go func() {
		defer close(packetChan)
		for {
			packet, err := s.source.NextPacket()
			if err == nil {
				packetChan <- packet
				continue
			}

			// Properly synchronizes termination process
			select {
			case <-s.exit:
				return
			default:
			}

			// Immediately retry for temporary network errors
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				continue
			}

			// Immediately retry for EAGAIN
			if err == syscall.EAGAIN {
				continue
			}

			// Immediately break for known unrecoverable errors
			if err == io.EOF || err == io.ErrUnexpectedEOF ||
				err == io.ErrNoProgress || err == io.ErrClosedPipe || err == io.ErrShortBuffer ||
				err == syscall.EBADF ||
				strings.Contains(err.Error(), "use of closed file") {
				break
			}

			// Sleep briefly and try again
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return packetChan
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
