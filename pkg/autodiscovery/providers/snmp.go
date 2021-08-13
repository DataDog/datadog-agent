// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

// SnmpProvider implements the ConfigProvider interface
// for the cluster check feature.
type SnmpProvider struct {
}

// NewSnmpProvider returns snmp configs
func NewSnmpProvider(cfg config.ConfigurationProviders) (ConfigProvider, error) {
	c := &SnmpProvider{}

	// Register in the cluster agent as soon as possible
	c.IsUpToDate(context.TODO()) //nolint:errcheck

	return c, nil
}

// String returns a string representation of the SnmpProvider
func (c *SnmpProvider) String() string {
	return names.SNMP
}

// IsUpToDate queries the cluster-agent to update its status and
// query if new configurations are available
func (c *SnmpProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return false, nil
}

// Collect retrieves configurations the cluster-agent dispatched to this agent
func (c *SnmpProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Warnf("[DEV] SNMP Config Provider Collect")
	return []integration.Config{}, nil
}

func init() {
	RegisterProvider("snmp", NewSnmpProvider)
}

// GetConfigErrors is not implemented for the SnmpProvider
func (c *SnmpProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
