// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"fmt"
	"io"
	"slices"

	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/network/netlink"
)

// DebugConntrackTable contains conntrack table data used for debugging NAT
type DebugConntrackTable struct {
	// Kind indicates if it is a table from conntrack/cached or conntrack/host, and whether
	// conntrack/cached was using ebpf or not.
	Kind    string
	RootNS  uint32
	Entries map[uint32][]netlink.DebugConntrackEntry
	// IsTruncated indicates we closed the netlink socket early - some clients have tens of thousands of
	// connections and if netlink takes too long, we just return partial results.
	IsTruncated bool
}

// WriteTo dumps the conntrack table in the style of `conntrack -L`.
// It sorts the output so that equivalent tables should result in the same text.
// If the output would result in more than maxEntries connections, it skips outputting the rest.
func (table *DebugConntrackTable) WriteTo(w io.Writer, maxEntries int) error {
	_, err := fmt.Fprintf(w, "conntrack dump, kind=%s rootNS=%d\n", table.Kind, table.RootNS)
	if err != nil {
		return err
	}

	namespaces := maps.Keys(table.Entries)
	slices.Sort(namespaces)

	totalEntries := 0
	for _, ns := range namespaces {
		totalEntries += len(table.Entries[ns])
	}

	suffix := "\n"
	if totalEntries > maxEntries {
		suffix = fmt.Sprintf(", capped to %d to reduce output size\n", maxEntries)
	}

	_, err = fmt.Fprintf(w, "totalEntries=%d%s", totalEntries, suffix)
	if err != nil {
		return err
	}

	// in this case the table itself is incomplete due to closing the netlink socket part-way
	if table.IsTruncated {
		_, err = fmt.Fprintln(w, "netlink table truncated due to response timeout, some entries may be missing")
	}

	// used to stop writing once we reach maxEntries
	totalEntriesWritten := 0

	for _, ns := range namespaces {
		_, err = fmt.Fprintf(w, "namespace %d, size=%d:\n", ns, len(table.Entries[ns]))
		if err != nil {
			return err
		}
		sortedEntries := slices.Clone(table.Entries[ns])
		slices.SortFunc(sortedEntries, func(a, b netlink.DebugConntrackEntry) int {
			return a.Compare(b)
		})
		for i, entry := range sortedEntries {
			// break out if we have written too much
			if totalEntriesWritten >= maxEntries {
				entriesLeft := len(sortedEntries) - i
				_, err = fmt.Fprintf(w, "<reached max entries, skipping remaining %d entries...>\n", entriesLeft)
				if err != nil {
					return err
				}
				break
			}

			// the entry roughly matches conntrack -L format
			_, err = fmt.Fprintln(w, entry.String())
			if err != nil {
				return err
			}
			totalEntriesWritten++
		}
	}

	return nil
}
