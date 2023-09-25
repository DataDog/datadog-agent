// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// WorkloadDumpResponse is used to dump the store content.
type WorkloadDumpResponse struct {
	Entities map[string]WorkloadEntity `json:"entities"`
}

// WorkloadEntity contains entity data.
type WorkloadEntity struct {
	Infos map[string]string `json:"infos"`
}

// Write writes the stores content in a given writer.
// Useful for agent's CLI and Flare.
func (wdr WorkloadDumpResponse) Write(writer io.Writer) {
	if writer != color.Output {
		color.NoColor = true
	}

	for kind, entities := range wdr.Entities {
		for entity, info := range entities.Infos {
			info = scrubMessage(info)
			fmt.Fprintf(writer, "\n=== Entity %s %s ===\n", color.GreenString(kind), color.GreenString(entity))
			fmt.Fprint(writer, info)
			fmt.Fprintln(writer, "===")
		}
	}
}

// Dump implements Store#Dump
func (s *store) Dump(verbose bool) WorkloadDumpResponse {
	workloadList := WorkloadDumpResponse{
		Entities: make(map[string]WorkloadEntity),
	}

	entityToString := func(entity Entity) (string, error) {
		var info string
		switch e := entity.(type) {
		case *Container:
			info = e.String(verbose)
		case *KubernetesPod:
			info = e.String(verbose)
		case *KubernetesNode:
			info = e.String(verbose)
		case *ECSTask:
			info = e.String(verbose)
		case *ContainerImageMetadata:
			info = e.String(verbose)
		case *Process:
			info = e.String(verbose)
		default:
			return "", fmt.Errorf("unsupported type %T", e)
		}

		return info, nil
	}

	s.storeMut.RLock()
	defer s.storeMut.RUnlock()

	for kind, store := range s.store {
		entities := WorkloadEntity{Infos: make(map[string]string)}
		for id, cachedEntity := range store {
			if verbose && len(cachedEntity.sources) > 1 {
				for source, entity := range cachedEntity.sources {
					info, err := entityToString(entity)
					if err != nil {
						log.Debugf("Ignoring entity %s: %v", entity.GetID().ID, err)
						continue
					}

					entities.Infos["source:"+string(source)+" id: "+id] = info
				}
			}

			e := cachedEntity.cached
			info, err := entityToString(e)
			if err != nil {
				log.Debugf("Ignoring entity %s: %v", e.GetID().ID, err)
				continue
			}

			entities.Infos[fmt.Sprintf("sources(merged):%v", cachedEntity.sortedSources)+" id: "+id] = info
		}

		workloadList.Entities[string(kind)] = entities
	}

	return workloadList
}

func scrubMessage(message string) string {
	msgScrubbed, err := scrubber.ScrubBytes([]byte(message))
	if err == nil {
		return string(msgScrubbed)
	}
	return message
}
