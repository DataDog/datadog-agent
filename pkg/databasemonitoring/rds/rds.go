// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build rds

// Package rds contains database-monitoring specific aurora discovery logic
package rds

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RDSListener listens for AWS RDS instances and creates service definitions for them.
type RDSListener struct {
	rdsServices map[string]listeners.Service
	stop        chan struct{}
}

// NewRDSListener creates a new RDSListener.
func NewRDSListener() (*RDSListener, error) {
	return &RDSListener{
		rdsServices: make(map[string]listeners.Service),
		stop:        make(chan struct{}),
	}, nil
}

// Listen starts the RDSListener.
func (l *RDSListener) Listen(ctx context.Context) {
	log.Info("Starting RDSListener")
	go l.refreshServices(ctx)
}

// Stop stops the RDSListener.
func (l *RDSListener) Stop() {
	close(l.stop)
	log.Info("RDSListener stopped")
}

// refreshServices periodically refreshes the list of RDS services.
func (l *RDSListener) refreshServices(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stop:
			return
		default:
			l.updateServices()
		}
	}
}

// updateServices fetches the list of RDS instances and updates the services.
func (l *RDSListener) updateServices() {
	// TODO: Implement logic to fetch RDS instances from AWS API
	log.Info("Fetching RDS instances...")
	// Example placeholder logic
	rdsInstances := []string{"desired-tag-key", "desired-tag-value"}

	for _, instance := range rdsInstances {
		if _, found := l.rdsServices[instance]; !found {
			service := listeners.NewService(instance, "rds")
			l.rdsServices[instance] = service
			listeners.NotifyServiceAdded(service)
			log.Infof("Added RDS service: %s", instance)
		}
	}

	// Remove services that no longer exist
	for instance := range l.rdsServices {
		if !contains(rdsInstances, instance) {
			listeners.NotifyServiceRemoved(l.rdsServices[instance])
			delete(l.rdsServices, instance)
			log.Infof("Removed RDS service: %s", instance)
		}
	}
}

// contains checks if a slice contains a specific string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
