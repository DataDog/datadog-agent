// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stream provides the autodiscovery stream config
package stream

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/proto"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config streams autodiscovery configs
func Config(ac autodiscovery.Component, out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	s := &scheduler{
		out:  out,
		done: make(chan error, 1),
	}

	// TODO: add a uuid to avoid name collision when there are concurrent rpc calls ?
	schedulerName := "remote"
	ac.AddScheduler(schedulerName, s, true)
	defer ac.RemoveScheduler(schedulerName)

	return <-s.done
}

type scheduler struct {
	out  pb.AgentSecure_AutodiscoveryStreamConfigServer
	done chan error
}

func (s *scheduler) Schedule(config []integration.Config) {
	s.handleEvent(config, pb.ConfigEventType_SCHEDULE)
}

func (s *scheduler) Unschedule(configs []integration.Config) {
	s.handleEvent(configs, pb.ConfigEventType_UNSCHEDULE)
}

func (s *scheduler) Stop() {
	close(s.done)
}

func (s *scheduler) handleEvent(configs []integration.Config, eventType pb.ConfigEventType) {
	protobufConfigs := protobufConfigFromAutodiscoveryConfigs(configs, eventType)

	err := grpc.DoWithTimeout(func() error {
		return s.out.Send(&pb.AutodiscoveryStreamResponse{
			Configs: protobufConfigs,
		})
	}, 1*time.Minute)

	if err != nil {
		log.Warnf("error sending %s autodiscovery event: %s", eventType.String(), err)
		s.done <- err
	}
}

func protobufConfigFromAutodiscoveryConfigs(config []integration.Config, eventType pb.ConfigEventType) []*pb.Config {
	res := make([]*pb.Config, 0, len(config))
	for _, c := range config {
		protobufConfig := proto.ProtobufConfigFromAutodiscoveryConfig(&c)
		protobufConfig.EventType = eventType
		res = append(res, protobufConfig)
	}
	return res
}
