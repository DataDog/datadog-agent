// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package hostname

import (
	"context"
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const configProvider = "configuration"

var (
	hostnameExpvars  = expvar.NewMap("hostname")
	hostnameProvider = expvar.String{}
	hostnameErrors   = expvar.Map{}
)

func init() {
	hostnameErrors.Init()
	hostnameExpvars.Set("provider", &hostnameProvider)
	hostnameExpvars.Set("errors", &hostnameErrors)
}

// providerCb is a generic function to grab the hostname and return it. currentHostname represents the hostname detected
// until now by previous providers.
type providerCb func(ctx context.Context, currentHostname string) (string, error)

type Provider struct {
	name string
	cb   providerCb

	// Should we stop going down the list of provider if this one is successful
	stopIfSucessful bool

	// expvarName is the name to use to store the error in expvar. This is legacy behavior to match the expvar name
	// from the previous hostname detection logic.
	expvarName string
}

// providerCatalog holds all the various kinds of hostname providers
//
// The order if this list matters:
// * Config (`hostname')
// * Config (`hostname_file')
// * Fargate
// * GCE
// * Azure
// * container (kube_apiserver, Docker, kubelet)
// * FQDN
// * OS hostname
// * EC2
var providerCatalog = []Provider{
	{
		name:            configProvider,
		cb:              fromConfig,
		stopIfSucessful: true,
		expvarName:      "configuration/environment",
	},
	{
		name:            "hostname_file",
		cb:              fromHostnameFile,
		stopIfSucessful: true,
		expvarName:      "configuration/environment",
	},
	{
		name:            "fargate",
		cb:              fromFarget,
		stopIfSucessful: true,
		expvarName:      "fargate",
	},
	{
		name:            "gce",
		cb:              fromGCE,
		stopIfSucessful: true,
		expvarName:      "gce",
	},
	{
		name:            "azure",
		cb:              fromAzure,
		stopIfSucessful: true,
		expvarName:      "azure",
	},

	// The following providers are coupled. Their behavior change following the result of the previous provider.
	// Therefore 'stopIfSucessful' is set to false.
	{
		name:            "fqdn",
		cb:              fromFQDN,
		stopIfSucessful: false,
		expvarName:      "fqdn",
	},
	{
		name:            "container",
		cb:              fromContainer,
		stopIfSucessful: false,
		expvarName:      "container",
	},
	{
		name:            "os",
		cb:              fromOS,
		stopIfSucessful: false,
		expvarName:      "os",
	},
	{
		name:            "ec2",
		cb:              fromEC2,
		stopIfSucessful: false,
		expvarName:      "aws",
	},
}

// HostnameData contains hostname and the hostname provider
type HostnameData struct {
	Hostname string
	Provider string
}

// IsConfigurationProvider returns true if the hostname was found throught the configuration file
func (h HostnameData) IsConfigurationProvider() bool {
	return h.Hostname == configProvider
}

func saveHostnameData(cacheHostnameKey string, hostname string, provider string) HostnameData {
	data := HostnameData{
		Hostname: hostname,
		Provider: provider,
	}

	cache.Cache.Set(cacheHostnameKey, data, cache.NoExpiration)
	if provider != "" || provider != "fargate" {
		hostnameProvider.Set(name)
		inventories.SetAgentMetadata(inventories.AgentHostnameSource, name)
	}
	return data
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it
func GetWithProvider(ctx context.Context) (HostnameData, error) {
	cacheHostnameKey := cache.BuildAgentKey("hostname")

	var err error
	var hostname string
	//providers := config.Datadog.GetStringSlice("hostname_providers")

	for providerName, provider := range providerCatalog {
		log.Debugf("trying to get hostname from '%s' provider", providerName)

		detectedHostname, err = provider.cb(ctx, hostname)
		if err != nil {
			expErr := new(expvar.String)
			expErr.Set(err.Error())
			hostnameErrors.Set(provider.expvarName, expErr)
			log.Debugf("Unable to get the hostname from '%s' provider: %s", providerName, err)
		}

		hostname = detectedHostname

		if Provider.stopIfSucessful {
			return saveHostnameData(cacheHostnameKey, hostname, provider.name), nil
		}
	}

	warnCanonicalHostname(ctx, hostname)

	if hostname != "" {
		return saveHostnameData(cacheHostnameKey, hostname, provider.name), nil
	}

	err = fmt.Errorf("unable to reliably determine the host name. You can define one in the agent config file or in your hosts file")
	expErr := new(expvar.String)
	expErr.Set(fmt.Sprintf(err.Error()))
	hostnameErrors.Set("all", expErr)
	return HostnameData{}, err
}

// Get returns the host name for the agent
func Get(ctx context.Context) (string, error) {
	hostname, _, err := GetWithProvider(ctx)
	return hostname, err
}
