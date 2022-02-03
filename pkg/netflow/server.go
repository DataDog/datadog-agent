// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package netflow

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/netsampler/goflow2/format"
	"net"
	"time"

	"github.com/netsampler/goflow2/utils"
	logrus "github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content *gosnmp.SnmpPacket
	Addr    *net.UDPAddr
}

// NfCollector manages an SNMPv2 trap listener.
type NfCollector struct {
	Addr          string
	config        *Config
	listener      *utils.StateNetFlow
	demultiplexer aggregator.Demultiplexer
}

var (
	serverInstance *NfCollector
)

// StartServer starts the global trap server.
func StartServer(demultiplexer aggregator.Demultiplexer) error {
	server, err := NewNetflowServer(demultiplexer)
	serverInstance = server
	return err
}

// StopServer stops the global trap server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.Stop()
		serverInstance = nil
	}
}

// IsRunning returns whether the trap server is currently running.
func IsRunning() bool {
	return serverInstance != nil
}

// NewNetflowServer configures and returns a running SNMP traps server.
func NewNetflowServer(demultiplexer aggregator.Demultiplexer) (*NfCollector, error) {
	config, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	listener, err := startSNMPv2Listener(config, demultiplexer)
	if err != nil {
		return nil, err
	}

	server := &NfCollector{
		listener:      listener,
		config:        config,
		demultiplexer: demultiplexer,
	}

	return server, nil
}

func startSNMPv2Listener(c *Config, demultiplexer aggregator.Demultiplexer) (*utils.StateNetFlow, error) {
	log.Warn("Starting Netflow Server")
	agg := demultiplexer.Aggregator()
	metricChan := agg.GetBufferedMetricsWithTsChannel()

	d := &Driver{
		MetricChan: metricChan,
	}
	format.RegisterFormatDriver("json", d)

	ctx := context.TODO()
	formatter, err := format.FindFormat(ctx, "json")
	if err != nil {
		return nil, err
	}

	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.TraceLevel)
	sNF := &utils.StateNetFlow{
		Format:    formatter,
		Logger:    logger,
	}
	hostname := c.BindHost
	port := c.Port
	reusePort := false
	go func() {
		log.Errorf("Starting FlowRoutine...")
		err = sNF.FlowRoutine(1, hostname, int(port), reusePort)
		log.Errorf("Exited FlowRoutine")
		if err != nil {
			log.Errorf("Error exiting FlowRoutine: %s", err)
		}
	}()

	return sNF, nil
}

// Stop stops the NfCollector.
func (s *NfCollector) Stop() {
	log.Infof("Stop listening on %s", s.config.Addr())
	stopped := make(chan interface{})

	go func() {
		log.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Shutdown()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		log.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}
}
