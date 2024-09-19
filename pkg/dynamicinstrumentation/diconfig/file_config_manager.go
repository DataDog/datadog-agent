// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/util"
)

func NewFileConfigManager(configFile string) (*ReaderConfigManager, error) {
	cm, err := NewReaderConfigManager()
	if err != nil {
		return nil, err
	}

	fw := util.NewFileWatcher(configFile)
	updateChan, err := fw.Watch()
	if err != nil {
		return nil, fmt.Errorf("failed to watch config file %s: %s", configFile, err)
	}

	go func() {
		for {
			select {
			case rawBytes := <-updateChan:
				cm.ConfigReader.Read(rawBytes)
			default:
			}
		}
	}()
	return cm, nil
}
