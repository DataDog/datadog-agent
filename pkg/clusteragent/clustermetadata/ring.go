// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"encoding/json"
	"hash/fnv"
	"sort"
	"time"
)

const OwnedNodesAnnotation = "clusteragent.datadoghq.com/owned-nodes"

// MemberInfo is a ring member's lease state, extracted from its Lease by
// the wiring layer. Name is the member identity: "namespace/pod-name"
// (see MemberID).
type MemberInfo struct {
	Name      string
	RenewedAt time.Time
	Duration  time.Duration
}

// AliveMembers returns the names of members whose lease is still valid (at now)
func AliveMembers(members []MemberInfo, now time.Time) []string {
	alive := make([]string, 0, len(members))
	for _, m := range members {
		if m.Name != "" && now.Before(m.RenewedAt.Add(m.Duration)) {
			alive = append(alive, m.Name)
		}
	}
	sort.Strings(alive)
	return alive
}

// Owner returns the member that owns node under rendezvous hashing.
// See: https://www.ietf.org/archive/id/draft-ietf-bess-weighted-hrw-00.html#name-hrw-introduction
func Owner(node string, members []string) string {
	owner := ""
	var best uint64
	for _, member := range members {
		score := rendezvousScore(member, node)
		// Tie-break on member name so the winner is deterministic
		if score > best || (score == best && member > owner) {
			best = score
			owner = member
		}
	}
	return owner
}

// Assign maps every node to its owning member.
func Assign(nodes []string, members []string) map[string]string {
	assignment := make(map[string]string, len(nodes))
	for _, node := range nodes {
		assignment[node] = Owner(node, members)
	}
	return assignment
}

// OwnedSets inverts an assignment into a map of members to the nodes they own.
func OwnedSets(nodes []string, members []string) map[string][]string {
	sets := make(map[string][]string, len(members))
	for _, member := range members {
		sets[member] = make([]string, 0)
	}
	for _, node := range nodes {
		owner := Owner(node, members)
		if owner != "" {
			sets[owner] = append(sets[owner], node)
		}
	}
	for member := range sets {
		sort.Strings(sets[member])
	}
	return sets
}

// MarshalOwnedNodes encodes a node set as a JSON-encoded string.
func MarshalOwnedNodes(nodes []string) (string, error) {
	sorted := make([]string, len(nodes))
	copy(sorted, nodes)
	sort.Strings(sorted)
	data, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalOwnedNodes decodes a JSON-encoded string into a list of nodes.
func UnmarshalOwnedNodes(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	var nodes []string
	if err := json.Unmarshal([]byte(value), &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// rendezvousScore is the hash of the (member, node) pair that ranks candidates for one node.
func rendezvousScore(member string, node string) uint64 {
	a := fnvHashString(member)
	b := fnvHashString(node)
	return mix64(a ^ (b * goldenRatio))
}

const goldenRatio uint64 = 0x9e3779b97f4a7c15

func fnvHashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
