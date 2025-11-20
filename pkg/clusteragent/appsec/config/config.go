// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package config handles the configuration of the AppSec Injection Proxy feature
package config

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

const (
	// AppsecProcessorResourceAnnotation is the annotation key used to store the address of the AppSec processor service
	AppsecProcessorResourceAnnotation = "appsec.datadoghq.com/processor"
	// AppsecProcessorProxyTypeAnnotation is the annotation key used to store the type of proxy used for AppSec injection
	AppsecProcessorProxyTypeAnnotation = "appsec.datadoghq.com/proxy-type"
)

// ProxyType represents the type of proxy supported by the AppSec Injection Proxy feature
// It has to be associated with both proxyMaps in proxies.go and the list of supported proxies in the Helm chart / Datadog Operator
type ProxyType string

const (
	// ProxyTypeEnvoyGateway represents the Envoy Gateway proxy type for appsec injection mode
	ProxyTypeEnvoyGateway ProxyType = "envoy-gateway"

	// ProxyTypeIstio represents the Istio proxy type for appsec injection mode
	ProxyTypeIstio ProxyType = "istio"
)

// AllProxyTypes is the list of all supported proxy types for appsec injection mode
var AllProxyTypes = []ProxyType{
	ProxyTypeEnvoyGateway,
	ProxyTypeIstio,
}

// Processor represents the configuration of the AppSec processor service that was deployed in the cluster
// to handle AppSec traffic from the injected proxies
type Processor struct {
	// Address is the address of the processor service
	// If empty, it will be derived from the ServiceName and Namespace fields
	// in the format <ServiceName>.<Namespace>.svc
	Address string
	// Port is the port of the processor service (typically 443)
	Port int
	// Namespace is the namespace where the processor service is deployed
	// It is used to derive the address of the processor service if the Address field is empty
	// If both Address and Namespace are empty, the resources namespace will be used
	// (the namespace where the Cluster Agent is deployed)
	Namespace string
	// ServiceName is the name of the processor service (required) // TODO: make it optional once we can deploy the processor ourselves
	// It is used to derive the address of the processor service if the Address field is empty
	ServiceName string
}

func (p Processor) String() string {
	address := p.Address
	if address == "" {
		address = p.ServiceName + "." + p.Namespace + ".svc"
	}
	return address + ":" + strconv.Itoa(p.Port)
}

// Product represents the configuration of the AppSec Injection Proxy agent feature
type Product struct {
	Enabled    bool
	Processor  Processor
	AutoDetect bool
	Proxies    map[ProxyType]struct{}
}

// Injection represent kubernetes specific configuration available for users to customize
// the resources created for the AppSec Injection Proxy feature
type Injection struct {
	Enabled           bool
	CommonLabels      map[string]string
	CommonAnnotations map[string]string
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration

	IstioNamespace string
}

// Config represents the configuration of the AppSec Injection Proxy feature passed down to [InjectionPattern] implementations
type Config struct {
	Injection
	Product
}

// FromComponent uses the datadog config.Component and returns a Config using default values when not set
func FromComponent(cfg config.Component, logger log.Component) Config {
	proxiesEnabled := cfg.GetStringSlice("appsec.proxy.proxies")
	proxiesMap := make(map[ProxyType]struct{}, len(proxiesEnabled))
	for _, p := range proxiesEnabled {
		proxyType := ProxyType(p)
		if !slices.Contains(AllProxyTypes, proxyType) {
			logger.Warnf("Proxy type %s is not supported for appsec injection, ignoring...", proxyType)
			continue
		}
		proxiesMap[proxyType] = struct{}{}
	}

	processor := Processor{
		Address:     cfg.GetString("appsec.proxy.processor.address"),
		Port:        cfg.GetInt("appsec.proxy.processor.port"),
		Namespace:   cfg.GetString("cluster_agent.appsec.injector.processor.service.namespace"),
		ServiceName: cfg.GetString("cluster_agent.appsec.injector.processor.service.name"),
	}

	if processor.Namespace == "" {
		// No namespace configured, default to the resources namespace
		// This is the namespace where the Cluster Agent is deployed
		// It is also the namespace where the processor service is deployed by default
		processor.Namespace = common.GetResourcesNamespace()
	}

	staticLabels := map[string]string{
		kubernetes.KubeAppComponentLabelKey:      "datadog-appsec-injector",
		kubernetes.KubeAppPartOfLabelKey:         "datadog",
		kubernetes.KubeAppManagedByLabelKey:      "datadog-cluster-agent",
		"appsec.datadoghq.com/injection-version": "v1",
	}

	staticAnnotations := map[string]string{
		AppsecProcessorResourceAnnotation: processor.String(),
	}

	maps.Copy(staticLabels, cfg.GetStringMapString("cluster_agent.appsec.injector.labels"))
	maps.Copy(staticAnnotations, cfg.GetStringMapString("cluster_agent.appsec.injector.annotations"))

	return Config{
		Product: Product{
			Enabled:    cfg.GetBool("appsec.proxy.enabled"),
			AutoDetect: cfg.GetBool("appsec.proxy.auto_detect"),
			Proxies:    proxiesMap,
			Processor:  processor,
		},
		Injection: Injection{
			Enabled:           cfg.GetBool("cluster_agent.appsec.injector.enabled"),
			CommonLabels:      staticLabels,
			CommonAnnotations: staticAnnotations,
			BaseBackoff:       cfg.GetDuration("cluster_agent.appsec.injector.base_backoff"),
			MaxBackoff:        cfg.GetDuration("cluster_agent.appsec.injector.max_backoff"),

			IstioNamespace: cfg.GetString("cluster_agent.appsec.injector.istio.namespace"),
		},
	}
}
