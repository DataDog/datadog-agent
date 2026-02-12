// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UDPListener listens for UDP datagrams and runs a single syslog UDPTailer.
type UDPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	conn             *net.UDPConn
	tailer           *tailer.UDPTailer
}

// NewUDPListener returns an initialized syslog UDPListener.
func NewUDPListener(pipelineProvider pipeline.Provider, source *sources.LogSource) *UDPListener {
	return &UDPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
	}
}

// Start opens the UDP socket and starts the syslog tailer.
func (l *UDPListener) Start() {
	log.Infof("Starting syslog UDP listener on port %d", l.source.Config.Port)
	err := l.startTailer()
	if err != nil {
		log.Errorf("Can't start syslog UDP listener on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
}

// Stop stops the tailer and closes the UDP connection.
func (l *UDPListener) Stop() {
	log.Infof("Stopping syslog UDP listener on port %d", l.source.Config.Port)
	if l.tailer != nil {
		l.tailer.Stop()
	}
}

// startTailer opens the UDP connection and starts the tailer.
func (l *UDPListener) startTailer() error {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	l.conn = conn
	l.tailer = tailer.NewUDPTailer(l.source, l.pipelineProvider.NextPipelineChan(), conn)
	l.tailer.Start()
	return nil
}
