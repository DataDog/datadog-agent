// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scap

import "fmt"

type SystemCharacteristics struct {
	SystemInfo SystemInfo `json:"system_info"`
	Objects    []Object   `json:"objects,omitempty"`
}

type SystemInfo struct {
	OsName          string `json:"os_name"`
	OsVersion       string `json:"os_version"`
	Architecture    string `json:"architecture"`
	PrimaryHostName string `json:"primary_host_name"`
}

type Object struct {
	ID    string `json:"id"`
	Items []Item `json:"items,omitempty"`
}

type Item struct {
	ID       string            `json:"id"`
	Messages map[string]string `json:"messages"`
}

func SysChar(doc *Document) (*SystemCharacteristics, error) {
	if doc.OvalSystemCharacteristics == nil {
		return nil, fmt.Errorf("OvalSystemCharacteristics is nil")
	}

	systemCharacteristics := SystemCharacteristics{}

	systemInfo := SystemInfo{
		OsName:          doc.OvalSystemCharacteristics.SystemInfo.OsName,
		OsVersion:       doc.OvalSystemCharacteristics.SystemInfo.OsVersion,
		Architecture:    doc.OvalSystemCharacteristics.SystemInfo.Architecture,
		PrimaryHostName: doc.OvalSystemCharacteristics.SystemInfo.PrimaryHostName,
	}
	systemCharacteristics.SystemInfo = systemInfo

	if doc.OvalSystemCharacteristics.CollectedObjects != nil {
		for _, object := range doc.OvalSystemCharacteristics.CollectedObjects.Object {
			o := Object{
				ID: string(object.Id),
			}
			if object.Flag == "complete" {
				if doc.OvalSystemCharacteristics.SystemData != nil {
					for _, reference := range object.Reference {
						for _, item := range doc.OvalSystemCharacteristics.SystemData.Item {
							if reference.ItemRef != item.Id {
								continue
							}
							i := Item{
								ID:       item.XMLName.Local,
								Messages: make(map[string]string, len(item.Message)),
							}
							for _, message := range item.Message {
								i.Messages[message.XMLName.Local] = message.Text
							}
							o.Items = append(o.Items, i)
						}
					}
				}
			}
			systemCharacteristics.Objects = append(systemCharacteristics.Objects, o)
		}
	}

	return &systemCharacteristics, nil
}
