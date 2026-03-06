// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostnameimpl

import (
	"context"
	"errors"
	"expvar"

	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configProviderName  = hostnamedef.ConfigProvider
	fargateProviderName = hostnamedef.FargateProvider
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

// providerCb is the function signature for hostname providers.
// currentHostname is the hostname detected so far by previous providers.
type providerCb func(ctx context.Context, cfg pkgconfigmodel.Reader, currentHostname string) (string, error)

// provider describes a single hostname detection strategy.
type provider struct {
	name string
	cb   providerCb

	// stopIfSuccessful indicates whether to stop querying further providers on success.
	stopIfSuccessful bool

	// expvarName is used as the key when recording errors in expvars (legacy naming).
	expvarName string
}

// getProviderCatalog returns the ordered list of hostname providers.
//
// Order matters:
//  1. Config (`hostname`)
//  2. Config (`hostname_file`)
//  3. Fargate/Sidecar (strips hostname for sidecar mode)
//  4. GCE
//  5. Azure
//  6. FQDN
//  7. Container (kube_apiserver → Docker → kubelet)
//  8. OS hostname
//  9. EC2 (standard or legacy resolution)
func getProviderCatalog(legacyHostnameResolution bool) []provider {
	catalog := []provider{
		{name: configProviderName, cb: fromConfig, stopIfSuccessful: true, expvarName: "'hostname' configuration/environment"},
		{name: "hostnameFile", cb: fromHostnameFile, stopIfSuccessful: true, expvarName: "'hostname_file' configuration/environment"},
		{name: fargateProviderName, cb: fromFargate, stopIfSuccessful: true, expvarName: "fargate"},
		{name: "gce", cb: fromGCE, stopIfSuccessful: true, expvarName: "gce"},
		{name: "azure", cb: fromAzure, stopIfSuccessful: true, expvarName: "azure"},
		// The following providers are coupled: their behavior changes based on what previous providers found.
		{name: "fqdn", cb: fromFQDN, stopIfSuccessful: false, expvarName: "fqdn"},
		{name: "container", cb: fromContainer, stopIfSuccessful: false, expvarName: "container"},
		{name: "os", cb: fromOS, stopIfSuccessful: false, expvarName: "os"},
	}
	if legacyHostnameResolution {
		catalog = append(catalog, provider{name: "aws", cb: fromEC2WithLegacyHostnameResolution, stopIfSuccessful: false, expvarName: "aws"})
	} else {
		catalog = append(catalog, provider{name: "aws", cb: fromEC2, stopIfSuccessful: false, expvarName: "aws"})
	}
	return catalog
}

// saveHostname caches the resolved hostname and updates the expvar provider display.
func saveHostname(cacheKey string, hostname string, providerName string, legacyHostnameResolution bool) hostnamedef.Data {
	data := hostnamedef.Data{
		Hostname: hostname,
		Provider: providerName,
	}
	cache.Cache.Set(cacheKey, data, cache.NoExpiration)
	// In Fargate sidecar mode the hostname is intentionally empty; don't expose it.
	if providerName != "" && providerName != fargateProviderName && !legacyHostnameResolution {
		hostnameProvider.Set(providerName)
	}
	return data
}

// getHostname resolves the hostname using the ordered provider catalog.
// Results are cached in pkg/util/cache under cacheKey.
func getHostname(ctx context.Context, cfg pkgconfigmodel.Reader, cacheKey string, legacyHostnameResolution bool) (hostnamedef.Data, error) {
	agentCacheKey := cache.BuildAgentKey(cacheKey)

	if cached, found := cache.Cache.Get(agentCacheKey); found {
		return cached.(hostnamedef.Data), nil
	}

	var hostname string
	var providerName string

	for _, p := range getProviderCatalog(legacyHostnameResolution) {
		log.Debugf("trying to get hostname from '%s' provider", p.name)

		detected, err := p.cb(ctx, cfg, hostname)
		if err != nil {
			expErr := new(expvar.String)
			expErr.Set(err.Error())
			hostnameErrors.Set(p.expvarName, expErr)
			log.Debugf("unable to get the hostname from '%s' provider: %s", p.name, err)
			continue
		}

		log.Debugf("hostname provider '%s' successfully found hostname '%s'", p.name, detected)
		hostname = detected
		providerName = p.name

		if p.stopIfSuccessful {
			log.Debugf("hostname provider '%s' succeeded, stopping here with hostname '%s'", p.name, detected)
			return saveHostname(agentCacheKey, hostname, providerName, legacyHostnameResolution), nil
		}
	}

	warnAboutFQDN(ctx, cfg, hostname)

	if hostname != "" {
		return saveHostname(agentCacheKey, hostname, providerName, legacyHostnameResolution), nil
	}

	err := errors.New("unable to reliably determine the host name. You can define one in the agent config file or in your hosts file")
	expErr := new(expvar.String)
	expErr.Set(err.Error())
	hostnameErrors.Set("all", expErr)
	return hostnamedef.Data{}, err
}
