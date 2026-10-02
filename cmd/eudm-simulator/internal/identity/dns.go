// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package identity

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

var errDNS = errors.New("cannot rewrite captured connection DNS")

// connectionsDNS changes only local lookup keys. Complete domain tables and
// domain offsets stay intact, including entries used solely by DNS query stats.
func (m *Map) connectionsDNS(connections *model.CollectorConnections) error {
	remote := map[string]bool{}
	for _, connection := range connections.Connections {
		if connection.Raddr != nil && !connection.IntraHost {
			remote[connection.Raddr.Ip] = true
		}
	}
	rewrite := func(entries map[string][]int32) (bool, error) {
		changed := false
		rewritten := make(map[string][]int32, len(entries))
		add := func(address string, refs []int32) error {
			if previous, exists := rewritten[address]; exists && !slices.Equal(previous, refs) {
				return errDNS
			}
			rewritten[address] = refs
			return nil
		}
		for address, refs := range entries {
			mapped := m.localIP(address)
			if err := add(mapped, refs); err != nil {
				return false, err
			}
			if mapped != address {
				changed = true
				if remote[address] {
					if err := add(address, refs); err != nil {
						return false, err
					}
				}
			}
		}
		if changed {
			clear(entries)
			for address, refs := range rewritten {
				entries[address] = refs
			}
		}
		return changed, nil
	}
	if len(connections.EncodedDNS) != 0 {
		entries, names, err := decodeDNSLookups(connections.EncodedDNS, 1)
		if err != nil {
			return err
		}
		changed, err := rewrite(entries)
		if err != nil {
			return err
		}
		if changed {
			values := make(map[string]*model.DNSEntry, len(entries))
			for address, refs := range entries {
				entry := &model.DNSEntry{}
				for _, ref := range refs {
					name, ok := names[ref]
					if !ok {
						return errDNS
					}
					entry.Names = append(entry.Names, name)
				}
				values[address] = entry
			}
			encoded, err := model.NewV1DNSEncoder().Encode(values)
			if err != nil {
				return errDNS
			}
			connections.EncodedDNS = encoded
		}
	}
	if len(connections.EncodedDnsLookups) != 0 {
		if telemetry.ValidateConnectionDNSV2(connections.EncodedDomainDatabase, connections.EncodedDnsLookups) != nil {
			return errDNS
		}
		entries, _, err := decodeDNSLookups(connections.EncodedDnsLookups, 2)
		if err != nil {
			return err
		}
		changed, err := rewrite(entries)
		if err != nil {
			return err
		}
		if changed {
			var offsets []int32
			indexes := map[int32]int32{}
			values := make(map[string]*model.DNSDatabaseEntry, len(entries))
			for address, refs := range entries {
				entry := &model.DNSDatabaseEntry{}
				for _, offset := range refs {
					index, ok := indexes[offset]
					if !ok {
						index = int32(len(offsets))
						indexes[offset] = index
						offsets = append(offsets, offset)
					}
					entry.NameOffsets = append(entry.NameOffsets, index)
				}
				values[address] = entry
			}
			encoded, err := model.NewV2DNSEncoder().EncodeMapped(values, offsets)
			if err != nil {
				return errDNS
			}
			connections.EncodedDnsLookups = encoded
		}
	}
	return nil
}

// Decode all lookup keys, including addresses absent from the current chunk.
// Re-encoding with the Agent encoder rebuilds hash buckets after a key changes.
func decodeDNSLookups(data []byte, version byte) (map[string][]int32, map[int32]string, error) {
	if len(data) < 3 || data[0] != version {
		return nil, nil, errDNS
	}
	buckets := int(binary.LittleEndian.Uint16(data[1:3]))
	if buckets == 0 {
		return nil, nil, errDNS
	}
	header := dnsCursor{data: data, at: 3}
	positionsLen, ok := header.number()
	if !ok {
		return nil, nil, errDNS
	}
	var namesLen uint64
	if version == 1 {
		namesLen, ok = header.number()
		if !ok {
			return nil, nil, errDNS
		}
	}
	middle, ok := header.number()
	if !ok || namesLen > uint64(len(data)-header.at) || positionsLen > uint64(len(data)-header.at)-namesLen {
		return nil, nil, errDNS
	}
	positions := dnsCursor{data: data[header.at : header.at+int(positionsLen)]}
	body := dnsCursor{data: data[header.at+int(positionsLen) : len(data)-int(namesLen)]}
	names := map[int32]string{}
	if version == 1 {
		nameData := dnsCursor{data: data[len(data)-int(namesLen):]}
		for nameData.at < len(nameData.data) {
			offset := nameData.at
			name, ok := nameData.text()
			if !ok || name == "" || offset > math.MaxInt32 || len(names) >= 65536 {
				return nil, nil, errDNS
			}
			names[int32(offset)] = name
		}
	}
	entries := map[string][]int32{}
	associations := 0
	for bucket := 0; bucket < buckets; bucket++ {
		if bucket == buckets/2 && middle != uint64(positions.at) {
			return nil, nil, errDNS
		}
		position, ok := positions.number()
		if !ok || position != uint64(body.at) {
			return nil, nil, errDNS
		}
		count, ok := body.number()
		if !ok || count > uint64(65536-len(entries)) {
			return nil, nil, errDNS
		}
		for range count {
			address, ok := body.text()
			if !ok || address == "" || len(address) > 64 {
				return nil, nil, errDNS
			}
			if _, exists := entries[address]; exists {
				return nil, nil, errDNS
			}
			count, ok := body.number()
			if !ok || count > uint64(65536-associations) {
				return nil, nil, errDNS
			}
			associations += int(count)
			refs := make([]int32, 0, int(count))
			for range count {
				offset, ok := body.number()
				if !ok || offset > math.MaxInt32 {
					return nil, nil, errDNS
				}
				if version == 1 {
					if _, found := names[int32(offset)]; !found {
						return nil, nil, errDNS
					}
				}
				refs = append(refs, int32(offset))
			}
			entries[address] = refs
		}
	}
	if body.at != len(body.data) || positions.at != len(positions.data) {
		return nil, nil, errDNS
	}
	return entries, names, nil
}

type dnsCursor struct {
	data []byte
	at   int
}

func (c *dnsCursor) number() (uint64, bool) {
	value, count := binary.Uvarint(c.data[c.at:])
	if count <= 0 {
		return 0, false
	}
	c.at += count
	return value, true
}
func (c *dnsCursor) text() (string, bool) {
	length, ok := c.number()
	if !ok || length > uint64(len(c.data)-c.at) {
		return "", false
	}
	value := string(c.data[c.at : c.at+int(length)])
	c.at += int(length)
	return value, true
}
