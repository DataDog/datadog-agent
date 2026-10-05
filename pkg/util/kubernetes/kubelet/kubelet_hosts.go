// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// connectionInfo contains potential kubelet's ips and hostnames
type connectionInfo struct {
	ips       []string
	hostnames []string
}

func getKubeletConnectionInfo() (hosts *connectionInfo, httpsPort int, httpPort int, pathPrefix string, err error) {
	httpsPort = pkgconfigsetup.Datadog().GetInt("kubernetes_https_kubelet_port")
	httpPort = pkgconfigsetup.Datadog().GetInt("kubernetes_http_kubelet_port")

	// Kubelet is unavailable, proxying calls through the APIServer (for instance EKS Fargate)
	if pkgconfigsetup.Datadog().GetBool("eks_fargate") {
		// Explicitly disable HTTP to reach APIServer
		httpPort = 0

		var httpsPort64 uint64
		httpsPort64, err = strconv.ParseUint(os.Getenv("KUBERNETES_SERVICE_PORT"), 10, 16)
		if err != nil {
			return nil, 0, 0, "", fmt.Errorf("unable to get APIServer port: %w", err)
		}

		httpsPort = int(httpsPort64)

		kubeletNodeName := pkgconfigsetup.Datadog().Get("kubernetes_kubelet_nodename")
		if kubeletNodeName == "" {
			return nil, 0, 0, "", errors.New("kubelet proxy mode enabled but nodename is empty - unable to query")
		}

		pathPrefix = fmt.Sprintf("/api/v1/nodes/%s/proxy", kubeletNodeName)
		apiServerIP := os.Getenv("KUBERNETES_SERVICE_HOST")

		hosts = &connectionInfo{
			ips: []string{apiServerIP},
		}
		log.Infof("EKS on Fargate mode detected, will proxy calls to the Kubelet through the APIServer at %s:%d%s", apiServerIP, httpsPort, pathPrefix)

		return hosts, httpsPort, httpPort, pathPrefix, nil
	}

	// Building a list of potential ips/hostnames to reach Kubelet
	kubeletHost := pkgconfigsetup.Datadog().GetString("kubernetes_kubelet_host")
	hosts = getPotentialKubeletHosts(kubeletHost)

	return hosts, httpsPort, httpPort, pathPrefix, nil
}

func getPotentialKubeletHosts(kubeletHost string) *connectionInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hosts := connectionInfo{ips: nil, hostnames: nil}
	if kubeletHost != "" {
		configIps, configHostnames := getKubeletHostFromConfig(ctx, kubeletHost)
		hosts.ips = append(hosts.ips, configIps...)
		hosts.hostnames = append(hosts.hostnames, configHostnames...)
		log.Debugf("Got potential kubelet connection info from config, ips: %v, hostnames: %v", configIps, configHostnames)
	}

	dockerIps, dockerHostnames := getKubeletHostFromDocker(ctx)
	hosts.ips = append(hosts.ips, dockerIps...)
	hosts.hostnames = append(hosts.hostnames, dockerHostnames...)
	log.Debugf("Got potential kubelet connection info from docker, ips: %v, hostnames: %v", dockerIps, dockerHostnames)

	dedupeConnectionInfo(&hosts)

	return &hosts
}

func getKubeletHostFromConfig(ctx context.Context, kubeletHost string) ([]string, []string) {
	var ips []string
	var hostnames []string
	if kubeletHost == "" {
		log.Debug("kubernetes_kubelet_host is not set")
		return ips, hostnames
	}

	log.Debugf("Trying to parse kubernetes_kubelet_host: %s", kubeletHost)
	kubeletIP := net.ParseIP(kubeletHost)
	if kubeletIP == nil {
		log.Debugf("Parsing kubernetes_kubelet_host: %s is a hostname, cached, trying to resolve it to ip...", kubeletHost)
		hostnames = append(hostnames, kubeletHost)
		ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, kubeletHost)
		if err != nil {
			log.Debugf("Cannot LookupIP hostname %s: %v", kubeletHost, err)
		} else {
			log.Debugf("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, ipAddrs)
			for _, ipAddr := range ipAddrs {
				ips = append(ips, ipAddr.IP.String())
			}
		}
	} else {
		log.Debugf("Parsed kubernetes_kubelet_host: %s is an address: %v, cached, trying to resolve it to hostname", kubeletHost, kubeletIP)
		ips = append(ips, kubeletIP.String())
		addrs, err := net.DefaultResolver.LookupAddr(ctx, kubeletHost)
		if err != nil {
			log.Debugf("Cannot LookupHost ip %s: %v", kubeletHost, err)
		} else {
			log.Debugf("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, addrs)
			hostnames = append(hostnames, addrs...)
		}
	}

	return ips, hostnames
}

func getKubeletHostFromDocker(ctx context.Context) ([]string, []string) {
	var ips []string
	var hostnames []string
	dockerHost, err := docker.GetHostname(ctx)
	if err != nil {
		log.Debugf("unable to get hostname from docker, make sure to set the kubernetes_kubelet_host option: %s", err)
		return ips, hostnames
	}

	log.Debugf("Trying to resolve host name %s provided by docker to ip...", dockerHost)
	hostnames = append(hostnames, dockerHost)
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, dockerHost)
	if err != nil {
		log.Debugf("Cannot resolve host name %s, cached, provided by docker to ip: %s", dockerHost, err)
	} else {
		log.Debugf("Resolved host name %s provided by docker to %v", dockerHost, ipAddrs)
		for _, ipAddr := range ipAddrs {
			ips = append(ips, ipAddr.IP.String())
		}
	}

	return ips, hostnames
}

func dedupeConnectionInfo(hosts *connectionInfo) {
	ipsKeys := make(map[string]bool)
	ips := []string{}
	for _, ip := range hosts.ips {
		if _, check := ipsKeys[ip]; !check {
			ipsKeys[ip] = true
			ips = append(ips, ip)
		}
	}

	hostnamesKeys := make(map[string]bool)
	hostnames := []string{}
	for _, hostname := range hosts.hostnames {
		if _, check := hostnamesKeys[hostname]; !check {
			hostnamesKeys[hostname] = true
			hostnames = append(hostnames, hostname)
		}
	}

	hosts.ips = ips
	hosts.hostnames = hostnames
}
