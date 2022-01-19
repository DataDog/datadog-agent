// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package netflow

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/json"
	// import
	_ "github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/json"
	// import
	_ "github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/protobuf"
	// import
	_ "github.com/DataDog/datadog-agent/pkg/netflow/goflow2/transport/file"
	// import
	_ "github.com/DataDog/datadog-agent/pkg/netflow/goflow2/transport/metric"
	"github.com/netsampler/goflow2/format"
	"github.com/netsampler/goflow2/transport"
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

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// NfCollector manages an SNMPv2 trap listener.
type NfCollector struct {
	Addr          string
	config        *Config
	listener      *utils.StateNetFlow
	packets       PacketsChannel
	demultiplexer aggregator.Demultiplexer
}

var (
	serverInstance *NfCollector
	startError     error
)

// StartServer starts the global trap server.
func StartServer(demultiplexer aggregator.Demultiplexer) error {
	server, err := NewNetflowServer(demultiplexer)
	serverInstance = server
	startError = err
	return err
}

// StopServer stops the global trap server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.Stop()
		serverInstance = nil
		startError = nil
	}
}

// IsRunning returns whether the trap server is currently running.
func IsRunning() bool {
	return serverInstance != nil
}

// GetPacketsChannel returns a channel containing all received trap packets.
func GetPacketsChannel() PacketsChannel {
	return serverInstance.packets
}

// NewNetflowServer configures and returns a running SNMP traps server.
func NewNetflowServer(demultiplexer aggregator.Demultiplexer) (*NfCollector, error) {
	config, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	packets := make(PacketsChannel, packetsChanSize)

	listener, err := startSNMPv2Listener(config, packets, demultiplexer)
	if err != nil {
		return nil, err
	}

	server := &NfCollector{
		listener:      listener,
		config:        config,
		packets:       packets,
		demultiplexer: demultiplexer,
	}

	return server, nil
}

func startSNMPv2Listener(c *Config, packets PacketsChannel, demultiplexer aggregator.Demultiplexer) (*utils.StateNetFlow, error) {
	log.Warn("Starting Netflow Server")
	agg := demultiplexer.Aggregator()
	metricChan := agg.GetBufferedMetricsWithTsChannel()

	//d := &metric.MetricDriver{
	//	Lock: &sync.RWMutex{},
	//	MetricChan: metricChan,
	//}
	//transport.RegisterTransportDriver("metric", d)

	d := &json.Driver{
		MetricChan: metricChan,
	}
	format.RegisterFormatDriver("json", d)

	ctx := context.TODO()
	formatter, err := format.FindFormat(ctx, "json")
	if err != nil {
		return nil, err
	}

	transporter, err := transport.FindTransport(ctx, "file")
	if err != nil {
		return nil, err
	}
	defer transporter.Close(ctx)

	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.TraceLevel)
	sNF := &utils.StateNetFlow{
		Format:    formatter,
		Transport: transporter,
		Logger:    logger,
	}
	hostname := "127.0.0.1"
	port := 9999
	reusePort := false
	err = sNF.FlowRoutine(1, hostname, int(port), reusePort)
	if err != nil {
		return nil, err
	}

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

	// Let consumers know that we will not be sending any more packets.
	close(s.packets)
}
