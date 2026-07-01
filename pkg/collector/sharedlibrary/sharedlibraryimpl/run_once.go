// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunOnce loads and runs a shared-library check once with in-memory configuration.
// The configuration is passed directly to the check FFI and is never written to disk.
func RunOnce(senderManager sender.SenderManager, libraryFolderPath string, checkName string, initConfig, instanceConfig integration.Data) error {
	log.Infof("sharedlibrary RunOnce: loading check %q from %s", checkName, libraryFolderPath)

	sharedLibraryLoader, err := ffi.NewSharedLibraryLoader(libraryFolderPath)
	if err != nil {
		return fmt.Errorf("creating shared library loader: %w", err)
	}

	loader := &CheckLoader{loader: sharedLibraryLoader}

	cfg := integration.Config{
		Name:       checkName,
		InitConfig: initConfig,
		Instances:  []integration.Data{instanceConfig},
		Source:     "datasecurity-rc",
		Provider:   "datasecurity",
	}

	ch, err := loader.Load(senderManager, cfg, instanceConfig, 0)
	if err != nil {
		return fmt.Errorf("loading %q check: %w", checkName, err)
	}
	defer ch.Cancel()

	log.Infof("sharedlibrary RunOnce: loaded check %q (id=%s), running", checkName, ch.ID())

	if _, err := senderManager.GetSender(ch.ID()); err != nil {
		return fmt.Errorf("getting sender for check %q: %w", ch.ID(), err)
	}
	defer senderManager.DestroySender(ch.ID())

	if err := ch.Run(); err != nil {
		return fmt.Errorf("running %q check: %w", checkName, err)
	}

	log.Infof("sharedlibrary RunOnce: check %q finished, committing sender", checkName)
	return nil
}
