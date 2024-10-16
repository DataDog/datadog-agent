// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package serverimpl implements the Server component interface
package serverimpl

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	server "github.com/DataDog/datadog-agent/comp/failover/server/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/haagent"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Requires defines the dependencies for the Server component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle

	Logger log.Component

	Demultiplexer demultiplexer.Component
}

// Provides defines the output of the Server component
type Provides struct {
	Comp server.Component

	Server *Server
}

// NewComponent creates a new Server component
func NewComponent(reqs Requires) (Provides, error) {

	reqs.Logger.Info("Start Failover Server")
	go startNewApiServer()

	senderInst, err := reqs.Demultiplexer.GetDefaultSender()
	if err != nil {
		return Provides{}, err
	}

	serverInst := &Server{
		logger: reqs.Logger,
		sender: senderInst,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			return serverInst.Start()
		},
		OnStop: func(_ context.Context) error {
			//serverInst.Stop() // TODO: implment
			return nil
		},
	})

	// TODO: Implement the serverInst component
	provides := Provides{
		Server: serverInst,
	}
	return provides, nil
}

// Server manages netflow listeners.
type Server struct {
	logger log.Component
	sender sender.Sender
}

func (s *Server) Start() error {
	if !haagent.IsInitialRolePrimary() {
		go s.checkHeartbeatLoop()
	}
	return nil
}

func (s *Server) checkHeartbeatLoop() {
	// TODO: use a class instead of function
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	tickerChan := ticker.C
	for {
		select {
		// stop sequence
		//case <-agg.stopChan:
		//	agg.flushLoopDone <- struct{}{}
		//	return
		// automatic flush sequence
		case t := <-tickerChan:
			s.logger.Infof("[checkHeartbeatLoop] Tick: %s", t.String())
			s.checkPrimary()
		}
	}
}

func (s *Server) checkPrimary() {
	configRole := pkgconfigsetup.Datadog().GetString("failover.role")
	s.logger.Warnf("[checkPrimary] Current Role:%s, Config Role:%s", haagent.GetRole(), configRole) // TODO: REMOVE ME

	if configRole == "standby" {
		s.handleFailover()
		s.logger.Warnf("[checkPrimary] Is Standby ") // TODO: REMOVE ME
	}
	clusterId := pkgconfigsetup.Datadog().GetString("ha_agent.cluster_id")
	agentHostname, _ := hostname.Get(context.TODO())
	role := haagent.GetRole()
	s.sender.Gauge("datadog.ha_agent.running", 1, "", []string{
		"cluster_id:" + clusterId,
		"host:" + agentHostname,
		"role:" + role,
	})
}

func (s *Server) handleFailover() {
	failoverPrimaryUrl := pkgconfigsetup.Datadog().GetString("failover.primary_url")

	urlstr := fmt.Sprintf("http://%s/health", failoverPrimaryUrl)
	s.logger.Warnf("[handleFailover] URL: %s", urlstr) // TODO: REMOVE ME

	var primaryIsUp bool

	resp, err := http.Get(urlstr)
	s.logger.Warnf("[handleFailover] DoGet resp: %v", resp) // TODO: REMOVE ME
	if err != nil {
		s.logger.Warnf("[handleFailover] DoGet err: `%s`", err.Error()) // TODO: REMOVE ME
		primaryIsUp = false
	} else {
		primaryIsUp = true
	}

	if primaryIsUp {
		s.logger.Warnf("[handleFailover] Primary is up")
		haagent.SetRole("standby")
	} else {
		s.logger.Warnf("[handleFailover] Primary is down")

		s.logger.Infof("[handleFailover] SetRole role=%s", "primary")
		haagent.SetRole("primary")
	}
}
