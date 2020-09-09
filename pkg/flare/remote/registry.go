// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const (
	expirationInterval = 5 * time.Minute
	purgeInterval      = 30 * time.Second
)

type RegisteredSource struct {
	Id      string
	Source  string
	Service string
	Env     string
}

var registrationMap = cache.NewCache(expirationInterval, purgeInterval)

func RegisterSource(id, source, service, env string) *RegisteredSource {
	//idempotent
	if item, ok := registrationMap.Get(id); ok {
		return item.(*RegisteredSource)
	}

	entry := &RegisteredSource{
		Id:      id,
		Source:  source,
		Service: service,
		Env:     env,
	}
	registrationMap.SetDefault(id, entry)
}

func GetSourcesForServiceAndEnv(service, env string) map[string]*RegisteredSource {
	sources := map[string]*RegisteredSource{}
	items := registrationMap.Items()
	for id, item := range items {
		if item.Expired() {
			continue
		}

		match := true
		source := item.Object.(*RegisteredSource)
		if service != "" && source.Service != service {
			match = false
		}
		if env != "" && source.Env != env {
			match = false
		}

		if match {
			sources[id] = source
		}
	}

	return sources
}
