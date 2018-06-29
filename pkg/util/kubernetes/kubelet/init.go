// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func isCertificatesConfigured() bool {
	return config.Datadog.GetString("kubelet_client_crt") != "" && config.Datadog.GetString("kubelet_client_key") != ""
}

func isTokenPathConfigured() bool {
	return config.Datadog.GetString("kubelet_auth_token_path") != ""
}

func buildTLSConfig(verifyTLS bool, caPath string) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if verifyTLS == false {
		log.Info("Skipping TLS verification")
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}

	if caPath == "" {
		log.Debug("kubelet_client_ca isn't configured: certificate authority must be trusted")
		return nil, nil
	}

	caPool, err := kubernetes.GetCertificateAuthority(caPath)
	if err != nil {
		return tlsConfig, err
	}
	tlsConfig.RootCAs = caPool
	tlsConfig.BuildNameToCertificate()
	return tlsConfig, nil
}

func getIPAddressesFromHostname(hostname string) ([]string, error) {
	if net.ParseIP(hostname) != nil {
		log.Debugf("This is already a resolved IP address: %s", hostname)
		return nil, fmt.Errorf("already a resolved IP address")
	}

	log.Debug("Trying to resolve the hostname provided %q")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		log.Debugf("Cannot LookupIP hostname %s: %v", hostname, err)
		return nil, err
	}
	var ipAddresses []string
	for _, elt := range ips {
		log.Debugf("Potential kubelet host: %q resolved from %q", elt.String(), hostname)
		ipAddresses = append(ipAddresses, elt.String())
	}
	return ipAddresses, nil
}

// potentialKubeletHostsFilter remove duplicates and returns IP addresses first
func potentialKubeletHostsFilter(potentialKubeletHosts []string) []string {
	potentialKubeletHostsSet := make(map[string]struct{})
	var ips []string
	var hostnames []string

	for _, elt := range potentialKubeletHosts {
		if _, ok := potentialKubeletHostsSet[elt]; ok {
			continue
		}
		potentialKubeletHostsSet[elt] = struct{}{}
		if net.ParseIP(elt) == nil {
			hostnames = append(hostnames, elt)
			continue
		}
		ips = append(ips, elt)
	}
	return append(ips, hostnames...)
}

func getKubeletPotentialHosts() []string {
	var potentialKubeletHosts []string

	// from configuration
	configuredKubeletHost := config.Datadog.GetString("kubernetes_kubelet_host")
	if configuredKubeletHost != "" {
		log.Infof("Potential kubelet host: %q provided by configuration kubernetes_kubelet_host", configuredKubeletHost)
		potentialKubeletHosts = append(potentialKubeletHosts, configuredKubeletHost)
		if !config.Datadog.GetBool("kubernetes_kubelet_host_autodetect") {
			log.Debugf("Skipping the discovery of potential kubelet hosts: kubernetes_kubelet_host_autodetect == false")
			return potentialKubeletHosts
		}
		potentialIPs, err := getIPAddressesFromHostname(configuredKubeletHost)
		if err == nil {
			potentialKubeletHosts = append(potentialKubeletHosts, potentialIPs...)
		}
	} else {
		log.Debugf("Empty configuration for kubernetes_kubelet_host")
	}

	// docker detection
	dockerHost, err := docker.HostnameProvider()
	if err == nil {
		log.Infof("Potential kubelet host: %q provided by docker", dockerHost)
		potentialKubeletHosts = append(potentialKubeletHosts, dockerHost)
		potentialIPs, err := getIPAddressesFromHostname(dockerHost)
		if err == nil {
			potentialKubeletHosts = append(potentialKubeletHosts, potentialIPs...)
		}
	} else {
		log.Debugf("Cannot use docker host as potential kubelet host: %v", err)
	}

	// ec2 detection
	ec2Host, err := ec2.GetHostname()
	if err == nil {
		log.Infof("Potential kubelet host: %q provided by ec2 metadata", ec2Host)
		potentialKubeletHosts = append(potentialKubeletHosts, ec2Host)
		potentialIPs, err := getIPAddressesFromHostname(ec2Host)
		if err == nil {
			potentialKubeletHosts = append(potentialKubeletHosts, potentialIPs...)
		}
	} else {
		log.Debugf("Cannot use ec2 host as potential kubelet host: %v", err)
	}

	// gce detection
	gceHost, err := gce.GetHostname()
	if err == nil {
		log.Infof("Potential kubelet host: %q provided by gce metadata", gceHost)
		potentialKubeletHosts = append(potentialKubeletHosts, gceHost)
		potentialIPs, err := getIPAddressesFromHostname(gceHost)
		if err == nil {
			potentialKubeletHosts = append(potentialKubeletHosts, potentialIPs...)
		}
	} else {
		log.Debugf("Cannot use gce host as potential kubelet host: %v", err)
	}

	potentialKubeletHosts = potentialKubeletHostsFilter(potentialKubeletHosts)
	log.Infof("Potential kubelet detected hosts are: %s", strings.Join(potentialKubeletHosts, ", "))
	return potentialKubeletHosts
}
