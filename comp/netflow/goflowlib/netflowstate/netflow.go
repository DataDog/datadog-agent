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
	"github.com/netsampler/goflow2/utils"
	"sync"
	"time"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/decoders/netflow/templates"
	"github.com/netsampler/goflow2/format"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/netsampler/goflow2/producer"
	"github.com/netsampler/goflow2/transport"
	"github.com/prometheus/client_golang/prometheus"
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

	mappedFieldsConfig map[uint16]config.Mapping
}

// NewStateNetFlow initializes a new Netflow/IPFIX producer, with the goflow default producer and the additional fields producer
func NewStateNetFlow(mappingConfs []config.Mapping) *StateNetFlow {
	return &StateNetFlow{
		ctx:                context.Background(),
		samplinglock:       &sync.RWMutex{},
		sampling:           make(map[string]producer.SamplingRateSystem),
		mappedFieldsConfig: mapFieldsConfig(mappingConfs),
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

	timeTrackStart := time.Now()
	msgDec, err := netflow.DecodeMessageContext(s.ctx, buf, key, netflow.TemplateWrapper{Ctx: s.ctx, Key: key, Inner: s.TemplateSystem})
	if err != nil {
		switch err.(type) {
		case *netflow.ErrorTemplateNotFound:
			utils.NetFlowErrors.With(
				prometheus.Labels{
					"router": key,
					"error":  "template_not_found",
				}).
				Inc()
		default:
			utils.NetFlowErrors.With(
				prometheus.Labels{
					"router": key,
					"error":  "error_decoding",
				}).
				Inc()
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
		timeDiff := fmsg.TimeReceived - fmsg.TimeFlowEnd

		message := common.FlowMessageWithAdditionalFields{
			FlowMessage: fmsg,
		}

		if additionalFields != nil {
			message.AdditionalFields = additionalFields[i]
		}

		utils.NetFlowTimeStatsSum.With(
			prometheus.Labels{
				"router":  key,
				"version": flowVersion(fmsg),
			}).
			Observe(float64(timeDiff))

		_, _, err := s.Format.Format(&message)

		if err != nil && s.Logger != nil {
			s.Logger.Error(err)
		}
	}

	timeTrackStop := time.Now()
	utils.DecoderTime.With(
		prometheus.Labels{
			"name": "NetFlow",
		}).
		Observe(float64((timeTrackStop.Sub(timeTrackStart)).Nanoseconds()) / 1000)

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
		utils.NetFlowStats.With(
			prometheus.Labels{
				"router":  exporterIP,
				"version": "9",
			}).
			Inc()
		for _, fs := range msgDec.FlowSets {
			s.sendFlowSetMetrics(fs, exporterIP, "9")
		}
	case netflow.IPFIXPacket:
		utils.NetFlowStats.With(
			prometheus.Labels{
				"router":  exporterIP,
				"version": "10",
			}).
			Inc()
		for _, fs := range msgDec.FlowSets {
			s.sendFlowSetMetrics(fs, exporterIP, "10")
		}
	}
}

func (s *StateNetFlow) sendFlowSetMetrics(fs any, exporterIP string, version string) {
	switch fsConv := fs.(type) {
	case netflow.TemplateFlowSet:
		labels := prometheus.Labels{
			"router":  exporterIP,
			"version": version,
			"type":    "TemplateFlowSet",
		}
		utils.NetFlowSetStatsSum.With(labels).Inc()
		utils.NetFlowSetRecordsStatsSum.With(labels).Add(float64(len(fsConv.Records)))
	case netflow.NFv9OptionsTemplateFlowSet:
		labels := prometheus.Labels{
			"router":  exporterIP,
			"version": version,
			"type":    "OptionsTemplateFlowSet",
		}
		utils.NetFlowSetStatsSum.With(labels).Inc()
		utils.NetFlowSetRecordsStatsSum.With(labels).Add(float64(len(fsConv.Records)))
	case netflow.OptionsDataFlowSet:
		labels := prometheus.Labels{
			"router":  exporterIP,
			"version": version,
			"type":    "OptionsDataFlowSet",
		}
		utils.NetFlowSetStatsSum.With(labels).Inc()
		utils.NetFlowSetRecordsStatsSum.With(labels).Add(float64(len(fsConv.Records)))
	case netflow.DataFlowSet:
		labels := prometheus.Labels{
			"router":  exporterIP,
			"version": version,
			"type":    "DataFlowSet",
		}
		utils.NetFlowSetStatsSum.With(labels).Inc()
		utils.NetFlowSetRecordsStatsSum.With(labels).Add(float64(len(fsConv.Records)))
	}
}

func flowVersion(fmsg *flowmessage.FlowMessage) string {
	switch fmsg.Type {
	case flowmessage.FlowMessage_NETFLOW_V9:
		return "9"
	case flowmessage.FlowMessage_IPFIX:
		return "10"
	default:
		return "unknown"
	}
}
