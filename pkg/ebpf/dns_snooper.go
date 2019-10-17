// +build linux_bpf

package ebpf

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	bpflib "github.com/iovisor/gobpf/elf"
)

const (
	dnsCacheTTL              = 3 * time.Minute
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
	packetBufferSize         = 100
)

var _ ReverseDNS = &SocketFilterSnooper{}

// SocketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type SocketFilterSnooper struct {
	tpacket      *afpacket.TPacket
	source       *gopacket.PacketSource
	socketFilter *bpflib.SocketFilter
	socketFD     int
	cache        *reverseDNSCache
	exit         chan struct{}
	wg           sync.WaitGroup
}

// NewSocketFilterSnooper returns a new SocketFilterSnooper
func NewSocketFilterSnooper(filter *bpflib.SocketFilter) (*SocketFilterSnooper, error) {
	tpacket, err := afpacket.NewTPacket(afpacket.OptPollTimeout(1 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("error creating raw socket: %s", err)
	}
	packetSrc := gopacket.NewPacketSource(tpacket, layers.LayerTypeEthernet)

	// The underlying socket file descriptor is private, hence the use of reflection
	socketFD := int(reflect.ValueOf(tpacket).Elem().FieldByName("fd").Int())
	if err := bpflib.AttachSocketFilter(filter, socketFD); err != nil {
		return nil, fmt.Errorf("error attaching filter to socket: %s", err)
	}

	cache := newReverseDNSCache(dnsCacheSize, dnsCacheTTL, dnsCacheExpirationPeriod)
	snooper := &SocketFilterSnooper{
		tpacket:      tpacket,
		source:       packetSrc,
		socketFilter: filter,
		socketFD:     socketFD,
		cache:        cache,
		exit:         make(chan struct{}),
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

// Resolve IPs to Names
func (s *SocketFilterSnooper) Resolve(connections []ConnectionStats) map[util.Address][]string {
	return s.cache.Get(connections, time.Now())
}

func (s *SocketFilterSnooper) GetStats() map[string]int64 {
	return s.cache.Stats()
}

// Close terminates the DNS traffic snooper as well as the underlying socket and the attached filter
func (s *SocketFilterSnooper) Close() {
	close(s.exit)
	s.wg.Wait()

	if err := bpflib.DetachSocketFilter(s.socketFilter, s.socketFD); err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}

	s.tpacket.Close()
	s.cache.Close()
}

func (s *SocketFilterSnooper) run(packets <-chan gopacket.Packet) {
	for packet := range packets {
		layer := packet.Layer(layers.LayerTypeDNS)
		if layer == nil {
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

		s.cache.Add(translation, time.Now())
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
		aliases       = [][]byte{}
		translation   = newTranslation(domainQueried)
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
			translation.add(util.AddressFromNetIP(record.IP))
			continue
		}
		for _, alias := range aliases {
			if bytes.Equal(alias, record.Name) {
				translation.add(util.AddressFromNetIP(record.IP))
				break
			}
		}
	}

	return translation
}
