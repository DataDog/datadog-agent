// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	gocache "github.com/patrickmn/go-cache"
)

const (
	expirationInterval = 5 * time.Minute
	purgeInterval      = 30 * time.Second
	maxTracers         = 10
)

var registrationMap = initRegistry()

func initRegistry() *gocache.Cache {
	c := cache.NewCache(expirationInterval, purgeInterval)
	c.OnEvicted(func(k string, v interface{}) {
		log.Warnf("source with key: %s was evicted", k)
	})
	return c
}

// Questions: should this schema be looser?
type RegisteredSource struct {
	Id      string
	Source  string
	Service string
	Env     string
}

func (rs *RegisteredSource) String() string {
	return fmt.Sprintf("id:%s\tsource:%s\tservice:%s\tenv:%s", rs.Id, rs.Source, rs.Service, rs.Env)
}

func RegisterSource(id, source, service, env string) *RegisteredSource {
	//idempotent
	if item, ok := registrationMap.Get(id); ok {
		// reset it to update the timestamp :(
		registrationMap.SetDefault(id, item)
		return item.(*RegisteredSource)
	}

	// Unfortunately go-cache does not update the timestamp
	// on get operations, so everything currently expires, this
	// results in increased allocations. Upstream PR in order
	// to add these functionalities, or write our own.
	entry := &RegisteredSource{
		Id:      id,
		Source:  source,
		Service: service,
		Env:     env,
	}
	registrationMap.SetDefault(id, entry)

	return entry
}

func GetSourceById(id string) (*RegisteredSource, bool) {
	source, err := registrationMap.Get(id)
	if s, ok := source.(*RegisteredSource); ok {
		return s, err
	}

	return nil, false
}

func GetSourcesByServiceAndEnv(service, env string) map[string]*RegisteredSource {
	sources := map[string]*RegisteredSource{}

	// only unexpired items are returned by Items()
	items := registrationMap.Items()
	for id, item := range items {
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

	// if service == "" && env == "" {
	// 	if len(sources) >= maxTracers {
	// 		return source, errors.New("Too many tracers in sources")

	// 	}
	// }

	return sources
}
