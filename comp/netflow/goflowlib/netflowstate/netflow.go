// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package netflowstate provides a Netflow state manager
// on top of goflow default producer, to allow additional fields collection.
package netflowstate

import (
	"bytes"
	"context"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/additionalfields"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/netsampler/goflow2/utils"
	"sync"
	"time"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	"github.com/netsampler/goflow2/format"
	"github.com/netsampler/goflow2/producer"
	"github.com/netsampler/goflow2/transport"
)

// StateNetFlow holds a NetflowV9/IPFIX producer
type StateNetFlow struct {
	stopper

	Format    format.FormatInterface
	Transport transport.TransportInterface
	Logger    utils.Logger

	samplinglock *sync.RWMutex
	sampling     map[string]producer.SamplingRateSystem

	Config       *producer.ProducerConfig
	configMapped *producer.ProducerConfigMapped

	TemplateSystem templates.TemplateInterface

	ctx context.Context

	sender             sender.Sender
	mappedFieldsConfig map[uint16]config.Mapping
}

// NewStateNetFlow initializes a new Netflow/IPFIX producer, with the goflow default producer and the additional fields producer
func NewStateNetFlow(mappingConfs []config.Mapping, sender sender.Sender) *StateNetFlow {
	return &StateNetFlow{
		ctx:                context.Background(),
		samplinglock:       &sync.RWMutex{},
		sampling:           make(map[string]producer.SamplingRateSystem),
		mappedFieldsConfig: mapFieldsConfig(mappingConfs),
		sender:             sender,
	}
}

// DecodeFlow decodes a flow into common.FlowMessageWithAdditionalFields
func (s *StateNetFlow) DecodeFlow(msg interface{}) error {
	pkt := msg.(utils.BaseMessage)
	buf := bytes.NewBuffer(pkt.Payload)

	key := pkt.Src.String()
	samplerAddress := pkt.Src
	if samplerAddress.To4() != nil {
		samplerAddress = samplerAddress.To4()
	}

	s.samplinglock.RLock()
	sampling, ok := s.sampling[key]
	s.samplinglock.RUnlock()
	if !ok {
		sampling = producer.CreateSamplingSystem()
		s.samplinglock.Lock()
		s.sampling[key] = sampling
		s.samplinglock.Unlock()
	}

	ts := uint64(time.Now().UTC().Unix())
	if pkt.SetTime {
		ts = uint64(pkt.RecvTime.UTC().Unix())
	}

	msgDec, err := netflow.DecodeMessageContext(s.ctx, buf, key, netflow.TemplateWrapper{Ctx: s.ctx, Key: key, Inner: s.TemplateSystem})
	if err != nil {
		switch err.(type) {
		case *netflow.ErrorTemplateNotFound:
			s.sender.Count(common.MetricPrefix+"processor.errors", 1, "", []string{"exporter_ip:" + key, "error:template_not_found"})
		default:
			s.sender.Count(common.MetricPrefix+"processor.errors", 1, "", []string{"exporter_ip:" + key, "error:error_decoding"})
		}
		return err
	}

	s.sendTelemetryMetrics(msgDec, key)

	flowMessageSet, err := producer.ProcessMessageNetFlowConfig(msgDec, sampling, s.configMapped)
	if err != nil {
		s.Logger.Errorf("failed to process netflow packet %s", err)
	}

	additionalFields, err := additionalfields.ProcessMessageNetFlowAdditionalFields(msgDec, s.mappedFieldsConfig)
	if err != nil {
		s.Logger.Errorf("failed to process additional fields %s", err)
	}

	for i, fmsg := range flowMessageSet {
		fmsg.TimeReceived = ts
		fmsg.SamplerAddress = samplerAddress

		message := common.FlowMessageWithAdditionalFields{
			FlowMessage: fmsg,
		}

		if additionalFields != nil {
			message.AdditionalFields = additionalFields[i]
		}

		_, _, err := s.Format.Format(&message)

		if err != nil && s.Logger != nil {
			s.Logger.Error(err)
		}
	}

	return nil
}

func (s *StateNetFlow) initConfig() {
	s.configMapped = producer.NewProducerConfigMapped(s.Config)
}

func mapFieldsConfig(mappingConfs []config.Mapping) map[uint16]config.Mapping {
	mappedFieldsConfig := make(map[uint16]config.Mapping)
	for _, conf := range mappingConfs {
		mappedFieldsConfig[conf.Field] = conf
	}
	return mappedFieldsConfig
}

// FlowRoutine starts a goflow flow routine
func (s *StateNetFlow) FlowRoutine(workers int, addr string, port int, reuseport bool) error {
	if err := s.start(); err != nil {
		return err
	}
	s.initConfig()
	return utils.UDPStoppableRoutine(s.stopCh, "NetFlow", s.DecodeFlow, workers, addr, port, reuseport, s.Logger)
}

func (s *StateNetFlow) sendTelemetryMetrics(msg any, exporterIP string) {
	switch msgDec := msg.(type) {
	case netflow.NFv9Packet:
		s.sender.Count(common.MetricPrefix+"processor.processed", 1, "", []string{"exporter_ip:" + exporterIP, "version:9"})
		for _, fs := range msgDec.FlowSets {
			s.sendFlowSetMetrics(fs, exporterIP, "9")
		}
	case netflow.IPFIXPacket:
		s.sender.Count(common.MetricPrefix+"processor.processed", 1, "", []string{"exporter_ip:" + exporterIP, "version:10"})
		for _, fs := range msgDec.FlowSets {
			s.sendFlowSetMetrics(fs, exporterIP, "10")
		}
	}
}

func (s *StateNetFlow) sendFlowSetMetrics(fs any, exporterIP string, version string) {
	switch fs.(type) {
	case netflow.TemplateFlowSet:
		s.sender.Count(common.MetricPrefix+"processor.flowsets", 1, "", []string{"exporter_ip:" + exporterIP, "version:" + version, "type:template_flow_set"})
	case netflow.NFv9OptionsTemplateFlowSet:
		s.sender.Count(common.MetricPrefix+"processor.flowsets", 1, "", []string{"exporter_ip:" + exporterIP, "version:" + version, "type:options_template_flow_set"})
	case netflow.OptionsDataFlowSet:
		s.sender.Count(common.MetricPrefix+"processor.flowsets", 1, "", []string{"exporter_ip:" + exporterIP, "version:" + version, "type:options_data_flow_set"})
	case netflow.DataFlowSet:
		s.sender.Count(common.MetricPrefix+"processor.flowsets", 1, "", []string{"exporter_ip:" + exporterIP, "version:" + version, "type:data_flow_set"})
	}
}
