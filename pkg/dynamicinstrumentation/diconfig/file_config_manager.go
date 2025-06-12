// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/util"
	"github.com/DataDog/datadog-agent/pkg/ebpf/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewFileConfigManager creates a new FileConfigManager
func NewFileConfigManager(pm process.Subscriber, configFile string) (*ReaderConfigManager, func(), error) {
	stopChan := make(chan bool)
	stop := func() {
		stopChan <- true
	}

	cm, err := NewReaderConfigManager(pm)
	if err != nil {
		return nil, stop, err
	}

	fw := util.NewFileWatcher(configFile)
	updateChan, err := fw.Watch()
	if err != nil {
		return nil, stop, fmt.Errorf("failed to watch config file %s: %s", configFile, err)
	}

	go func() {
		for {
			select {
			case rawBytes := <-updateChan:
				_, err := cm.ConfigWriter.Write(rawBytes)
				if err != nil {
					log.Errorf("Error writing config file %s: %s", configFile, err)
				}
			case <-stopChan:
				log.Info("stopping file config manager")
				fw.Stop()
				return
			}
		}
	}()
	return cm, stop, nil
}
