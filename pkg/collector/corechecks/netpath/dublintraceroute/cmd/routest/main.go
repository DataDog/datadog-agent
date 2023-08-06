/* SPDX-License-Identifier: BSD-2-Clause */

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"time"

	"github.com/florianl/go-nfqueue"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

var (
	flagConfig        = flag.String("c", "", "Config file")
	flagQueue         = flag.Int("q", -1, "NfQueue ID")
	flagIfname        = flag.String("i", "", "Interface name to send packets through. If not specified, packets are not forced through a specific interface")
	flagNoDefaultDrop = flag.Bool("no-default-drop", false, "Do not drop non-matching packets")
)

// Config is a configuration to match probe packets with custom replies.
type Config []Probe

// Probe is the probe packet we want to match.
type Probe struct {
	Src     *net.IP `json:"src,omitempty"`
	Dst     net.IP  `json:"dst"`
	SrcPort *uint16 `json:"src_port,omitempty"`
	DstPort uint16  `json:"dst_port"`
	TTL     uint8   `json:"ttl"`
	Reply   Reply   `json:"reply"`
}

// Reply is what we want to reply for a given hop.
type Reply struct {
	Src      net.IP  `json:"src"`
	Dst      *net.IP `json:"dst,omitempty"`
	IcmpType uint8   `json:"icmp_type"`
	IcmpCode uint8   `json:"icmp_code"`
	Payload  []byte  `json:"payload,omitempty"`
}

func loadConfig(buf []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(buf, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func main() {
	log.Level = logrus.DebugLevel
	flag.Parse()

	if *flagQueue < 0 {
		log.Fatalf("Must specify a nfqueue ID")
	}
	if *flagConfig == "" {
		log.Fatal("Empty config")
	}
	data, err := ioutil.ReadFile(*flagConfig)
	if err != nil {
		log.Fatalf("Cannot read file %s: %v", *flagConfig, err)
	}
	cfg, err := loadConfig(data)
	if err != nil {
		log.Fatalf("Cannot load config file %s: %v", *flagConfig, err)
	}
	log.Infof("Loaded configuration: \n%s", data)

	nfqConfig := nfqueue.Config{
		NfQueue:      uint16(*flagQueue),
		MaxPacketLen: 0xffff,
		MaxQueueLen:  0xff,
		Copymode:     nfqueue.NfQnlCopyPacket,
		ReadTimeout:  5 * time.Second,
	}
	nfq, err := nfqueue.Open(&nfqConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := nfq.Close(); err != nil {
			log.Debugf("Failed to close NfQueue: %v", err)
		}
	}()

	ctx := context.Background()
	fn := func(a nfqueue.Attribute) (ret int) {
		// TODO IPv6
		verdict := nfqueue.NfDrop
		header, payload, err := forgeReplyv4(cfg, *a.Payload)
		if err != nil {
			if err == ErrNoMatch {
				log.Infof("Packet not matching")
				if *flagNoDefaultDrop {
					verdict = nfqueue.NfAccept
				}
			} else {
				log.Warningf("Failed to forge reply: %v", err)
			}
			goto end
		}
		if err := Send4(*flagIfname, header, payload); err != nil {
			log.Warningf("send4 failed: %v", err)
			goto end
		}
	end:
		if err := nfq.SetVerdict(*a.PacketID, verdict); err != nil {
			log.Warningf("SetVerdict failed: %v", err)
		}
		return 0
	}
	if err := nfq.Register(ctx, fn); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
}
