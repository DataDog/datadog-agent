// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	AppsecProcessorResourceAnnotation  = "appsec.datadoghq.com/processor"
	AppsecProcessorProxyTypeAnnotation = "appsec.datadoghq.com/proxy-type"
)

type ProxyType string

const (
	ProxyTypeEnvoyGateway ProxyType = "envoy-gateway"
)

var proxyList = []ProxyType{
	ProxyTypeEnvoyGateway,
}

type Processor struct {
	Address     string
	Port        int
	Namespace   string
	ServiceName string
}

func (p Processor) String() string {
	address := p.Address
	if address == "" {
		address = p.ServiceName + "." + p.Namespace + ".svc"
	}
	return address + ":" + string(rune(p.Port))
}

type Product struct {
	Enabled    bool
	Processor  Processor
	AutoDetect bool
	Proxies    map[ProxyType]struct{}
}

type Injection struct {
	Enabled     bool
	BaseBackoff time.Duration
	MaxBackoff  time.Duration

	CommonLabels      map[string]string
	CommonAnnotations map[string]string
}

type Config struct {
	Injection
	Product
}

func FromComponent(cfg config.Component) Config {
	proxiesEnabled := cfg.GetStringSlice("appsec.proxy.proxies")
	proxiesMap := make(map[ProxyType]struct{}, len(proxiesEnabled))
	for _, p := range proxiesEnabled {
		if !slices.Contains(proxyList, ProxyType(p)) {
			continue
		}
		proxiesMap[ProxyType(p)] = struct{}{}
	}

	processor := Processor{
		Address:     cfg.GetString("appsec.proxy.processor.address"),
		Port:        cfg.GetInt("appsec.proxy.processor.port"),
		Namespace:   cfg.GetString("cluster_agent.appsec.injector.processor.service.namespace"),
		ServiceName: cfg.GetString("cluster_agent.appsec.injector.processor.service.name"),
	}

	return Config{
		Product: Product{
			Enabled:    cfg.GetBool("appsec.proxy.enabled"),
			AutoDetect: cfg.GetBool("appsec.proxy.auto_detect"),
			Proxies:    proxiesMap,
			Processor:  processor,
		},
		Injection: Injection{
			Enabled:     cfg.GetBool("cluster_agent.appsec.injector.enabled"),
			BaseBackoff: cfg.GetDuration("cluster_agent.appsec.injector.base_backoff"),
			MaxBackoff:  cfg.GetDuration("cluster_agent.appsec.injector.max_backoff"),
			CommonLabels: map[string]string{
				kubernetes.KubeAppManagedByLabelKey: "datadog-cluster-agent",
				kubernetes.KubeAppComponentLabelKey: "datadog-appsec-injector",
				kubernetes.KubeAppPartOfLabelKey:    "datadog",
			},
			CommonAnnotations: map[string]string{
				AppsecProcessorResourceAnnotation: processor.String(),
			},
		},
	}
}
