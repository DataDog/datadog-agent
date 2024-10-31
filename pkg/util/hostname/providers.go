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

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configProviderName  = hostnameinterface.ConfigProvider
	fargateProviderName = hostnameinterface.FargateProvider
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

// List of hostname providers
var (
	configProvider = provider{
		name:             configProviderName,
		cb:               fromConfig,
		stopIfSuccessful: true,
		expvarName:       "'hostname' configuration/environment",
	}

	hostnameFileProvider = provider{
		name:             "hostnameFile",
		cb:               fromHostnameFile,
		stopIfSuccessful: true,
		expvarName:       "'hostname_file' configuration/environment",
	}

	fargateProvider = provider{
		name:             fargateProviderName,
		cb:               fromFargate,
		stopIfSuccessful: true,
		expvarName:       "fargate",
	}

	gceProvider = provider{
		name:             "gce",
		cb:               fromGCE,
		stopIfSuccessful: true,
		expvarName:       "gce",
	}

	azureProvider = provider{
		name:             "azure",
		cb:               fromAzure,
		stopIfSuccessful: true,
		expvarName:       "azure",
	}

	// The following providers are coupled. Their behavior changes depending on the result of the previous provider.
	// Therefore 'stopIfSuccessful' is set to false.
	fqdnProvider = provider{
		name:             "fqdn",
		cb:               fromFQDN,
		stopIfSuccessful: false,
		expvarName:       "fqdn",
	}

	containerProvider = provider{
		name:             "container",
		cb:               fromContainer,
		stopIfSuccessful: false,
		expvarName:       "container",
	}

	osProvider = provider{
		name:             "os",
		cb:               fromOS,
		stopIfSuccessful: false,
		expvarName:       "os",
	}

	ec2Provider = provider{
		name:             "aws", // ie EC2
		cb:               fromEC2,
		stopIfSuccessful: false,
		expvarName:       "aws",
	}

	ec2HostnameResolutionProvider = provider{
		name:             "aws",
		cb:               fromEC2WithLegacyHostnameResolution,
		stopIfSuccessful: false,
		expvarName:       "aws",
	}
)

// providerCatalog holds all the various kinds of hostname providers
//
// The order if this list matters:
// * Config (`hostname')
// * Config (`hostname_file')
// * Fargate
// * GCE
// * Azure
// * FQDN
// * container (kube_apiserver, Docker, kubelet)
// * OS hostname
// * EC2
func getProviderCatalog(legacyHostnameResolution bool) []provider {
	providerCatalog := []provider{
		configProvider,
		hostnameFileProvider,
		fargateProvider,
		gceProvider,
		azureProvider,
		fqdnProvider,
		containerProvider,
		osProvider,
	}

	if legacyHostnameResolution {
		providerCatalog = append(providerCatalog, ec2HostnameResolutionProvider)
	} else {
		providerCatalog = append(providerCatalog, ec2Provider)
	}

	return providerCatalog
}

func saveHostname(cacheHostnameKey string, hostname string, providerName string, legacyHostnameResolution bool) Data {
	data := Data{
		Hostname: hostname,
		Provider: providerName,
	}

	cache.Cache.Set(cacheHostnameKey, data, cache.NoExpiration)
	// We don't have a hostname on fargate. 'fromFargate' will return an empty hostname and we don't want to show it
	// in the status page.
	if providerName != "" && providerName != fargateProviderName && !legacyHostnameResolution {
		hostnameProvider.Set(providerName)
	}
	return data
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it
func GetWithProvider(ctx context.Context) (Data, error) {
	return getHostname(ctx, "hostname", false)
}

// GetWithProviderLegacyResolution returns the hostname for the Agent and the provider that was used to retrieve it without using IMDSv2 and MDI
func GetWithProviderLegacyResolution(ctx context.Context) (Data, error) {
	// If the user has set the ec2_prefer_imdsv2 then IMDSv2 is used by default by the user, `legacy_resolution_hostname` is not needed for the transition
	// If the user has set the ec2_imdsv2_transition_payload_enabled then IMDSv2 is used by default by the agent, `legacy_resolution_hostname` is needed for the transition
	if pkgconfigsetup.Datadog().GetBool("ec2_prefer_imdsv2") || !pkgconfigsetup.Datadog().GetBool("ec2_imdsv2_transition_payload_enabled") {
		return Data{}, nil
	}
	return getHostname(ctx, "legacy_resolution_hostname", true)
}

func getHostname(ctx context.Context, keyCache string, legacyHostnameResolution bool) (Data, error) {
	cacheHostnameKey := cache.BuildAgentKey(keyCache)

	// first check if we have a hostname cached
	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		return cacheHostname.(Data), nil
	}

	var err error
	var hostname string
	var providerName string

	for _, p := range getProviderCatalog(legacyHostnameResolution) {
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
			return saveHostname(cacheHostnameKey, hostname, p.name, legacyHostnameResolution), nil

		}
	}

	warnAboutFQDN(ctx, hostname)

	if hostname != "" {
		return saveHostname(cacheHostnameKey, hostname, providerName, legacyHostnameResolution), nil
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
