// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && arm64

package diconfig

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for {
			select {
			case rawBytes := <-updateChan:
				cm.ConfigWriter.Write(rawBytes)
			case <-c:
				log.Info("stopping file config manager")
				fw.Stop()
				return
			}
		}
	}()
	return cm, nil
}
