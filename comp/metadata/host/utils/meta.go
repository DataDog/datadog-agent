// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
)

var (
	metaCacheKey = cache.BuildAgentKey("host", "utils", "meta")
)

// Meta is the metadata nested under the meta key
type Meta struct {
	SocketHostname string   `json:"socket-hostname"`
	Timezones      []string `json:"timezones"`
	SocketFqdn     string   `json:"socket-fqdn"`
	EC2Hostname    string   `json:"ec2-hostname"`
	Hostname       string   `json:"hostname"`
	HostAliases    []string `json:"host_aliases"`
	InstanceID     string   `json:"instance-id"`
	AgentHostname  string   `json:"agent-hostname,omitempty"`
	ClusterName    string   `json:"cluster-name,omitempty"`
}

// GetMetaFromCache returns the metadata information about the host from the cache and returns it, if the cache is
// empty, then it queries the information directly
func GetMetaFromCache(ctx context.Context, conf config.ConfigReader) *Meta {
	res, _ := cache.Get[*Meta](
		metaCacheKey,
		func() (*Meta, error) {
			return GetMeta(ctx, conf), nil
		},
	)
	return res
}

// GetMeta returns the metadata information about the host and refreshes the cache
func GetMeta(ctx context.Context, conf config.ConfigReader) *Meta {
	osHostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()
	ec2Hostname, _ := ec2.GetHostname(ctx)
	instanceID, _ := ec2.GetInstanceID(ctx)

	var agentHostname string

	hostnameData, _ := hostname.GetWithProvider(ctx)
	if conf.GetBool("hostname_force_config_as_canonical") && hostnameData.FromConfiguration() {
		agentHostname = hostnameData.Hostname
	}

	m := &Meta{
		SocketHostname: osHostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(osHostname),
		EC2Hostname:    ec2Hostname,
		HostAliases:    cloudproviders.GetHostAliases(ctx),
		InstanceID:     instanceID,
		AgentHostname:  agentHostname,
	}

	if finalClusterName := kubelet.GetMetaClusterNameText(ctx, osHostname); finalClusterName != "" {
		m.ClusterName = finalClusterName
	}

	cache.Cache.Set(metaCacheKey, m, cache.NoExpiration)
	return m
}
