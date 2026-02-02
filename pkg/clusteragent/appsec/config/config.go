// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package config handles the configuration of the AppSec Injection Proxy feature
package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

const (
	// AppsecProcessorResourceAnnotation is the annotation key used to store the address of the AppSec processor service
	AppsecProcessorResourceAnnotation = "appsec.datadoghq.com/processor"
	// AppsecProcessorProxyTypeAnnotation is the annotation key used to store the type of proxy used for AppSec injection
	AppsecProcessorProxyTypeAnnotation = "appsec.datadoghq.com/proxy-type"
	// AppsecInjectionVersionAnnotation is the version annotation key used to track the injector version
	AppsecInjectionVersionAnnotation = "appsec.datadoghq.com/injection-version"
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
// to handle AppSec traffic from the injected proxies. Is not used when sidecar mode is enabled.
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
	// ServiceName is the name of the processor service (required for EXTERNAL mode)
	// It is used to derive the address of the processor service if the Address field is empty
	ServiceName string
}

// Sidecar contains configuration for SIDECAR mode. Is not used when external mode is used
type Sidecar struct {
	// Image is the container image for the processor (e.g., "ghcr.io/datadog/dd-trace-go/service-extensions-callout")
	Image string
	// ImageTag is the image tag (e.g., "latest")
	ImageTag string
	// Port is the port for the sidecar to listen on (default: 8080)
	Port int
	// HealthPort is the health check port (default: 8081)
	HealthPort int

	// Resource requirements
	CPURequest    string // e.g., "100m"
	CPULimit      string // e.g., "200m"
	MemoryRequest string // e.g., "128Mi"
	MemoryLimit   string // e.g., "256Mi"

	// Environment variables
	BodyParsingSizeLimit string // Default: "10000000" (10MB)
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
	Enabled bool

	// AutoDetect tries to find all currently installed integration that could be enabled and enables them.
	AutoDetect bool

	// Proxies are manually set by the user to be enabled if the auto-detection is disabled
	Proxies map[ProxyType]struct{}

	// Mode determines how the processor is deployed
	// "external" - requires manual deployment, proxies call via service
	// "sidecar" - automatically injected as sidecar in proxy pods
	Mode InjectionMode

	// Sidecar contains configuration for SIDECAR mode
	Sidecar Sidecar

	// Processor contains configuration for the EXTERNAL mode
	Processor Processor
}

// Injection represent kubernetes specific configuration available for users to customize
// the resources created for the AppSec Injection Proxy feature
type Injection struct {
	Enabled           bool
	CommonLabels      map[string]string
	CommonAnnotations map[string]string
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration

	// IstioNamespace is used to determine where we will inject the `EnvoyFilter` object to make it global to the cluster.
	IstioNamespace string
}

// Config represents the configuration of the AppSec Injection Proxy feature passed down to [InjectionPattern] implementations
type Config struct {
	Injection
	Product
}

// validateSidecarConfig validates that required sidecar configuration fields are set
func validateSidecarConfig(config Sidecar) error {
	var errs []error

	if config.Image == "" {
		errs = append(errs, errors.New("sidecar image is required"))
	}

	if config.Port <= 0 || config.Port > 65535 {
		errs = append(errs, fmt.Errorf("sidecar.port must be between 1 and 65535, got: %d", config.Port))
	}

	if config.HealthPort <= 0 || config.HealthPort > 65535 {
		errs = append(errs, fmt.Errorf("sidecar.health_port must be between 1 and 65535, got: %d", config.HealthPort))
	}

	if config.Port == config.HealthPort {
		errs = append(errs, fmt.Errorf("sidecar.port and sidecar.health_port cannot be the same: %d", config.Port))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
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
		processor.Namespace = namespace.GetResourcesNamespace()
	}

	// Populate Sidecar for SIDECAR mode
	sidecarConfig := Sidecar{
		Image:                cfg.GetString("admission_controller.appsec.sidecar.image"),
		ImageTag:             cfg.GetString("admission_controller.appsec.sidecar.image_tag"),
		Port:                 cfg.GetInt("admission_controller.appsec.sidecar.port"),
		HealthPort:           cfg.GetInt("admission_controller.appsec.sidecar.health_port"),
		CPURequest:           cfg.GetString("admission_controller.appsec.sidecar.resources.requests.cpu"),
		CPULimit:             cfg.GetString("admission_controller.appsec.sidecar.resources.limits.cpu"),
		MemoryRequest:        cfg.GetString("admission_controller.appsec.sidecar.resources.requests.memory"),
		MemoryLimit:          cfg.GetString("admission_controller.appsec.sidecar.resources.limits.memory"),
		BodyParsingSizeLimit: cfg.GetString("admission_controller.appsec.sidecar.body_parsing_size_limit"),
	}

	staticLabels := map[string]string{
		kubernetes.KubeAppComponentLabelKey: "datadog-appsec-injector",
		kubernetes.KubeAppPartOfLabelKey:    "datadog",
		kubernetes.KubeAppManagedByLabelKey: "datadog-cluster-agent",
		AppsecInjectionVersionAnnotation:    "v2",
	}

	staticAnnotations := make(map[string]string, 1)

	mode := InjectionMode(strings.ToLower(cfg.GetString("cluster_agent.appsec.injector.mode")))
	switch mode {
	default:
		logger.Warnf("Invalid appsec proxy injection mode: %q (defaults to sidecar mode)", mode)
		mode = InjectionModeSidecar
		fallthrough
	case InjectionModeSidecar:
		staticAnnotations[AppsecProcessorResourceAnnotation] = "localhost"
		// Validate required sidecar configuration
		if err := validateSidecarConfig(sidecarConfig); err != nil {
			logger.Errorf("Invalid sidecar configuration: %v", err)
		}
	case InjectionModeExternal:
		staticAnnotations[AppsecProcessorResourceAnnotation] = processor.String()
		// Validate required external configuration
		if processor.ServiceName == "" {
			logger.Error("processor.service.name is required for EXTERNAL mode")
		}
	}

	maps.Copy(staticLabels, cfg.GetStringMapString("cluster_agent.appsec.injector.labels"))
	maps.Copy(staticAnnotations, cfg.GetStringMapString("cluster_agent.appsec.injector.annotations"))

	return Config{
		Product: Product{
			Enabled:    cfg.GetBool("appsec.proxy.enabled"),
			AutoDetect: cfg.GetBool("appsec.proxy.auto_detect"),
			Proxies:    proxiesMap,
			Processor:  processor,
			Mode:       mode,
			Sidecar:    sidecarConfig,
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
