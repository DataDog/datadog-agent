// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

// Package results provides the data structures to represent the results of a
// traceroute run
package results

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// IP represents some information from the IP header.
type IP struct {
	SrcIP net.IP `json:"src"`
	DstIP net.IP `json:"dst"`
	ID    uint16 `json:"id"`
	TTL   uint8  `json:"ttl"`
}

// UDP represents some information from the UDP header.
type UDP struct {
	SrcPort uint16 `json:"sport"`
	DstPort uint16 `json:"dport"`
}

// ICMP represents some information from the ICMP header.
type ICMP struct {
	Code        uint8           `json:"code"`
	Type        uint8           `json:"type"`
	Description string          `json:"description"`
	Extensions  []ICMPExtension `json:"extensions"`
	MPLSLabels  []MPLSLabel     `json:"mpls_labels"`
}

// ICMPExtension represents the ICMP extension header.
type ICMPExtension struct {
	Class   uint8  `json:"class"`
	Type    uint8  `json:"type"`
	Payload []byte `json:"payload"`
	Size    uint8  `json:"size"`
}

// MPLSLabel represents an MPLS label in an ICMP header.
type MPLSLabel struct {
	BottomOfStack uint8  `json:"bottom_of_stack"`
	Experimental  uint8  `json:"experimental"`
	Label         uint32 `json:"label"`
	TTL           uint8  `json:"ttl"`
}

// UnixUsec is UNIX time in the form sec.usec
type UnixUsec time.Time

// UnmarshalJSON deserializes a seconds.microseconds timestamp into an UnixUsec
// object. The timestamp can be optionally surrounded by double quotes.
func (um *UnixUsec) UnmarshalJSON(b []byte) error {
	s := string(b)
	// strip quotes, if any
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	split := strings.Split(string(s), ".")
	if len(split) != 2 {
		return fmt.Errorf("invalid timestamp %s", s)
	}
	sec, err := strconv.ParseInt(split[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid seconds in timestamp %s: %v", s, err)
	}
	if len(split[1]) > 6 {
		// truncate string, we only want down to microseconds
		split[1] = split[1][:6]
	}
	dec, err := strconv.ParseInt(split[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid decimal string in timestamp %s: %v", s, err)
	}
	if dec > 999999 {
		return fmt.Errorf("invalid decimal string in timestamp %s: too large", s)
	}
	// right-pad decimal string with zeros
	decStr := split[1]
	if len(split[1]) < 6 {
		decStr += strings.Repeat("0", 6-len(split[1]))
	}
	// now that it's correctly padded, parse it again
	usec, err := strconv.ParseInt(strings.TrimRight(decStr, " "), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid microseconds in timestamp %s: %v", s, err)
	}

	if usec > 999999 {
		return fmt.Errorf("invalid microseconds in timestamp %s: too large", s)
	}
	*um = UnixUsec(time.Unix(sec, usec*1000))
	return nil
}

// MarshalJSON serializes an UnixUsec object into a seconds.microseconds
// representation.
func (um UnixUsec) MarshalJSON() ([]byte, error) {
	u := time.Time(um).UnixNano() / 1000
	return []byte(fmt.Sprintf("%d.%06d", u/1e6, u%1e6)), nil
}

// Packet represents some information of a sent or received packet.
type Packet struct {
	Timestamp UnixUsec `json:"timestamp"`
	IP        IP       `json:"ip"`
	UDP       *UDP     `json:"udp,omitempty"`
	ICMP      *ICMP    `json:"icmp,omitempty"`
	// TODO add TCP, HTTP, DNS
}

// Probe holds information about a dublin-traceroute probe.
type Probe struct {
	Flowhash             uint16  `json:"flowhash"`
	IsLast               bool    `json:"is_last"`
	Name                 string  `json:"name"`
	NATID                uint16  `json:"nat_id"`
	RttUsec              uint64  `json:"rtt_usec"`
	Sent                 Packet  `json:"sent"`
	Received             *Packet `json:"received"`
	ZeroTTLForwardingBug bool    `json:"zerottl_forwarding_bug"`
}

// Results is the main container type for a dublin-traceroute set of results.
type Results struct {
	Flows      map[uint16][]Probe `json:"flows"`
	compressed bool
}

func (r *Results) compress() {
	if r.compressed {
		return
	}
	for k, v := range r.Flows {
		for idx, e := range v {
			if e.IsLast {
				r.Flows[k] = r.Flows[k][:idx+1]
				break
			}
		}
	}
	r.compressed = true
}

// ToJSON encodes a Results object to a JSON string.
func (r *Results) ToJSON(compress bool, indent string) (string, error) {
	if compress {
		if !r.compressed {
			r.compress()
		}
	}
	b, err := json.MarshalIndent(r, "", indent)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(b), nil
}
