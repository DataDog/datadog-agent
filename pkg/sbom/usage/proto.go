// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usage

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// IndexToProto converts an index to its wire form. The file names stay behind:
// a consumer matches the hash of the path it resolved, so sending the text would
// cost several times the bytes for nothing.
func IndexToProto(idx *Index) *pb.Index {
	out := &pb.Index{
		ScanId:     string(idx.Scan),
		Generation: idx.Generation,
		IndexId:    idx.IndexID,
		Status:     pb.IndexStatus(idx.Status),
		Components: make([]*pb.Component, 0, len(idx.Components)),
		Refs:       idx.Refs,
		Hashes:     idx.Hashes,
	}
	refCounts := make(map[string]int, len(idx.Components))
	indexed := make([]bool, len(idx.Components))
	for _, comp := range idx.Components {
		if comp.BOMRef != "" {
			refCounts[comp.BOMRef]++
		}
	}
	for _, ref := range idx.Refs {
		if uint64(ref) < uint64(len(indexed)) {
			indexed[ref] = true
		}
	}

	for i, comp := range idx.Components {
		// Core-side reportability is derived from the final BOM ref rather than
		// trusted as separate state. The bool is retained on the wire-side model.
		reportable := indexed[i] && refCounts[comp.BOMRef] == 1
		out.Components = append(out.Components, &pb.Component{
			Purl:        comp.Purl,
			Name:        comp.Name,
			Version:     comp.Version,
			Epoch:       int32(comp.Epoch),
			Release:     comp.Release,
			SrcVersion:  comp.SrcVersion,
			SrcEpoch:    int32(comp.SrcEpoch),
			SrcRelease:  comp.SrcRelease,
			Application: comp.Application,
			Reportable:  reportable,
		})
	}
	return out
}

// IndexFromProto converts an index back from its wire form.
func IndexFromProto(in *pb.Index) *Index {
	idx := &Index{
		Scan:       ScanID(in.GetScanId()),
		Generation: in.GetGeneration(),
		IndexID:    in.GetIndexId(),
		Status:     Status(in.GetStatus()),
		Components: make([]Component, 0, len(in.GetComponents())),
		Refs:       in.GetRefs(),
		Hashes:     in.GetHashes(),
	}

	for _, comp := range in.GetComponents() {
		// Before reportable and IndexID existed, every PURL-bearing component
		// could be reported. Preserve that behavior when talking to an old core
		// agent; a new index states reportability explicitly.
		reportable := comp.GetReportable()
		if in.GetIndexId() == "" && comp.GetPurl() != "" {
			reportable = true
		}
		idx.Components = append(idx.Components, Component{
			Purl:        comp.GetPurl(),
			Name:        comp.GetName(),
			Version:     comp.GetVersion(),
			Epoch:       int(comp.GetEpoch()),
			Release:     comp.GetRelease(),
			SrcVersion:  comp.GetSrcVersion(),
			SrcEpoch:    int(comp.GetSrcEpoch()),
			SrcRelease:  comp.GetSrcRelease(),
			Application: comp.GetApplication(),
			Reportable:  reportable,
		})
	}
	return idx
}

// ReportToProto converts a usage report to its wire form.
func ReportToProto(report *Report) *pb.UsageReport {
	out := &pb.UsageReport{
		ScanId:     string(report.Scan),
		Generation: report.Generation,
		IndexId:    report.IndexID,
		Usage:      make([]*pb.Usage, 0, len(report.Usage)),
	}
	for _, u := range report.Usage {
		out.Usage = append(out.Usage, &pb.Usage{
			Ref:          u.Ref,
			LastSeenUnix: u.LastSeen.Unix(),
			Suid:         u.Suid,
			AsRoot:       u.AsRoot,
		})
	}
	return out
}

// ReportFromProto converts a usage report back from its wire form.
func ReportFromProto(in *pb.UsageReport) *Report {
	report := &Report{
		Scan:       ScanID(in.GetScanId()),
		Generation: in.GetGeneration(),
		IndexID:    in.GetIndexId(),
		Usage:      make([]Usage, 0, len(in.GetUsage())),
	}
	for _, u := range in.GetUsage() {
		var lastSeen time.Time
		if unix := u.GetLastSeenUnix(); unix > 0 {
			lastSeen = time.Unix(unix, 0)
		}
		report.Usage = append(report.Usage, Usage{
			Ref:      u.GetRef(),
			LastSeen: lastSeen,
			Suid:     u.GetSuid(),
			AsRoot:   u.GetAsRoot(),
		})
	}
	return report
}

// CapabilitiesToProto converts a capability set to its wire form.
func CapabilitiesToProto(c Capabilities) *pb.Capabilities {
	return &pb.Capabilities{
		ContainerImage: c.ContainerImage,
		Container:      c.Container,
		Host:           c.Host,
	}
}

// CapabilitiesFromProto converts a capability set back from its wire form.
func CapabilitiesFromProto(in *pb.Capabilities) Capabilities {
	return Capabilities{
		ContainerImage: in.GetContainerImage(),
		Container:      in.GetContainer(),
		Host:           in.GetHost(),
	}
}
