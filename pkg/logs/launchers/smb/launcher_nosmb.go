// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !smb || goexperiment.systemcrypto || goexperiment.boringcrypto || requirefips

// Package smb reports sources of type smb as unsupported in the builds of the
// Agent that do not include the SMB log source: every build but the full,
// non-FIPS Agent on Linux, Windows and macOS (see launcher.go). Without it, smb
// sources would stay pending.
package smb

import (
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// errFIPS is the status of smb sources in FIPS builds.
	errFIPS = errors.New("smb log sources are not supported in FIPS builds of the Agent: SMB NTLMv2 authentication requires MD4, HMAC-MD5 and RC4, which are not FIPS-approved algorithms")
	// errNotIncluded is the status of smb sources in the other builds.
	errNotIncluded = errors.New("this build of the Agent does not include the SMB log source, which is available in the full Agent on Linux, Windows and macOS")
)

// Launcher gives every smb source an error status.
type Launcher struct {
	builtForFIPS func() bool // test seam

	done     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once

	// latest is the last source added for each configuration entry, owned by
	// the run goroutine. See replace.
	latest map[configEntry]*sources.LogSource
}

// NewLauncher returns a Launcher. It takes the arguments of the full Agent's
// launcher, which it does not use.
func NewLauncher(_ time.Duration, _ ...Option) *Launcher {
	return &Launcher{
		builtForFIPS: fips.BuiltForFIPS,
		done:         make(chan struct{}),
		latest:       make(map[configEntry]*sources.LogSource),
	}
}

// Start implements launchers.Launcher.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, _ pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
	err := errNotIncluded
	if l.builtForFIPS() {
		err = errFIPS
	}
	added, removed := sourceProvider.SubscribeForType(config.SMBType, l.done, l.done)
	l.stopped = make(chan struct{})
	go func() {
		defer close(l.stopped)
		for {
			select {
			case source := <-added:
				l.replace(source)
				log.Warnf("Not tailing smb source %s: %v", source.Name, err)
				source.Status().Error(err)
			case source := <-removed:
				if entry, ok := configEntryOf(source); ok && l.latest[entry] == source {
					delete(l.latest, entry)
				}
			case <-l.done:
				return
			}
		}
	}()
}

// replace hides from agent status the source that source replaces, as the full
// Agent's launcher does: the one created from the same configuration entry.
// Autodiscovery schedules a conf.d config again without unscheduling it when
// its secrets are refreshed, so without this every refresh would list the same
// error once more.
func (l *Launcher) replace(source *sources.LogSource) {
	entry, ok := configEntryOf(source)
	if !ok {
		return
	}
	if previous, ok := l.latest[entry]; ok && previous != source {
		previous.HideFromStatus()
	}
	l.latest[entry] = source
}

// Stop implements launchers.Launcher. It is safe to call when Start never ran.
func (l *Launcher) Stop() {
	l.stopOnce.Do(func() {
		close(l.done)
		if l.stopped != nil {
			<-l.stopped
		}
	})
}
