// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configProvider  = "configuration"
	fargateProvider = "fargate"
)

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

type provider struct {
	name string
	cb   providerCb

	// Should we stop going down the list of provider if this one is successful
	stopIfSuccessful bool

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
var providerCatalog = []provider{
	{
		name:             configProvider,
		cb:               fromConfig,
		stopIfSuccessful: true,
		expvarName:       "'hostname' configuration/environment",
	},
	{
		name:             "hostnameFile",
		cb:               fromHostnameFile,
		stopIfSuccessful: true,
		expvarName:       "'hostname_file' configuration/environment",
	},
	{
		name:             fargateProvider,
		cb:               fromFargate,
		stopIfSuccessful: true,
		expvarName:       "fargate",
	},
	{
		name:             "gce",
		cb:               fromGCE,
		stopIfSuccessful: true,
		expvarName:       "gce",
	},
	{
		name:             "azure",
		cb:               fromAzure,
		stopIfSuccessful: true,
		expvarName:       "azure",
	},

	// The following providers are coupled. Their behavior changes depending on the result of the previous provider.
	// Therefore 'stopIfSuccessful' is set to false.
	{
		name:             "fqdn",
		cb:               fromFQDN,
		stopIfSuccessful: false,
		expvarName:       "fqdn",
	},
	{
		name:             "container",
		cb:               fromContainer,
		stopIfSuccessful: false,
		expvarName:       "container",
	},
	{
		name:             "os",
		cb:               fromOS,
		stopIfSuccessful: false,
		expvarName:       "os",
	},
	{
		name:             "aws", // ie EC2
		cb:               fromEC2,
		stopIfSuccessful: false,
		expvarName:       "aws",
	},
}

// FromConfiguration returns true if the hostname was found through the configuration file
func (h Data) FromConfiguration() bool {
	return h.Provider == configProvider
}

func saveHostname(cacheHostnameKey string, hostname string, providerName string) Data {
	data := Data{
		Hostname: hostname,
		Provider: providerName,
	}

	cache.Cache.Set(cacheHostnameKey, data, cache.NoExpiration)
	// We don't have a hostname on fargate. 'fromFargate' will return an empty hostname and we don't want to show it
	// in the status page.
	if providerName != "" && providerName != fargateProvider {
		hostnameProvider.Set(providerName)
		inventories.SetAgentMetadata(inventories.AgentHostnameSource, providerName)
	}
	return data
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it
func GetWithProvider(ctx context.Context) (Data, error) {
	cacheHostnameKey := cache.BuildAgentKey("hostname")

	// first check if we have a hostname cached
	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		return cacheHostname.(Data), nil
	}

	var err error
	var hostname string
	var providerName string

	for _, p := range providerCatalog {
		log.Debugf("trying to get hostname from '%s' provider", p.name)

		detectedHostname, err := p.cb(ctx, hostname)
		if err != nil {
			expErr := new(expvar.String)
			expErr.Set(err.Error())
			hostnameErrors.Set(p.expvarName, expErr)
			log.Debugf("unable to get the hostname from '%s' provider: %s", p.name, err)
			continue
		}

		log.Debugf("hostname provider '%s' successfully found hostname '%s'", p.name, detectedHostname)
		hostname = detectedHostname
		providerName = p.name

		if p.stopIfSuccessful {
			log.Debugf("hostname provider '%s' succeeded, stoping here with hostname '%s'", p.name, detectedHostname)
			return saveHostname(cacheHostnameKey, hostname, p.name), nil
		}
	}

	warnAboutFQDN(ctx, hostname)

	if hostname != "" {
		return saveHostname(cacheHostnameKey, hostname, providerName), nil
	}

	err = fmt.Errorf("unable to reliably determine the host name. You can define one in the agent config file or in your hosts file")
	expErr := new(expvar.String)
	expErr.Set(err.Error())
	hostnameErrors.Set("all", expErr)
	return Data{}, err
}

// Get returns the host name for the agent
func Get(ctx context.Context) (string, error) {
	data, err := GetWithProvider(ctx)
	return data.Hostname, err
}
