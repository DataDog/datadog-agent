// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func printDiscardee(discarderDumpFile *os.File, discardee, discardeeParams string, discardeeCount int) {
	fmt.Fprintf(discarderDumpFile, "%d: %s\n", discardeeCount, discardee)
	fmt.Fprintf(discarderDumpFile, "%d: %s\n", discardeeCount, discardeeParams)
}

func (p *Probe) printDiscarderStats(dumpFile *os.File, mapName string) error {
	statsMap, _, err := p.manager.GetMap(mapName)
	if err != nil {
		return err
	}

	var key uint32
	var dStats discarderStats

	mapInfo, err := statsMap.Info()
	if err != nil {
		return err
	}

	statsCount := 0
	maxEntries := int(mapInfo.MaxEntries)
	for entries := statsMap.Iterate(); entries.Next(&key, &dStats); {
		statsCount++
		fmt.Fprintf(dumpFile, "%d: %+v\n", key, dStats)
		if statsCount == maxEntries {
			log.Infof("Stat count has reached max stat map size")
			break
		}
	}

	return nil
}
