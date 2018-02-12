// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package util

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

const maxLength = 255

var (
	validHostnameRfc1123 = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
	localhostIdentifiers = []string{
		"localhost",
		"localhost.localdomain",
		"localhost6.localdomain6",
		"ip6-localhost",
	}
)

// ValidHostname determines whether the passed string is a valid hostname.
// In case it's not, the returned error contains the details of the failure.
func ValidHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("host name is empty")
	} else if isLocal(hostname) {
		return fmt.Errorf("%s is a local hostname", hostname)
	} else if len(hostname) > maxLength {
		return fmt.Errorf("name exceeded the maximum length of %d characters", maxLength)
	} else if !validHostnameRfc1123.MatchString(hostname) {
		return fmt.Errorf("%s is not RFC1123 compliant", hostname)
	}
	return nil
}

// check whether the name is in the list of local hostnames
func isLocal(name string) bool {
	name = strings.ToLower(name)
	for _, val := range localhostIdentifiers {
		if val == name {
			return true
		}
	}
	return false
}

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return hostname
	}

	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			ip, err := ipv4.MarshalText()
			if err != nil {
				return hostname
			}
			hosts, err := net.LookupAddr(string(ip))
			if err != nil || len(hosts) == 0 {
				return hostname
			}
			return hosts[0]
		}
	}
	return hostname
}

// GetHostname retrieve the host name for the Agent, trying to query these
// environments/api, in order:
// * GCE
// * Docker
// * kubernetes
// * os
// * EC2
func GetHostname() (string, error) {
	cacheHostnameKey := cache.BuildAgentKey("hostname")
	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		return cacheHostname.(string), nil
	}

	var hostName string
	var err error

	// try the name provided in the configuration file
	name := config.Datadog.GetString("hostname")
	err = ValidHostname(name)
	if err == nil {
		cache.Cache.Set(cacheHostnameKey, name, cache.NoExpiration)
		return name, err
	}

	log.Debugf("Unable to get the hostname from the config file: %s", err)
	log.Debug("Trying to determine a reliable host name automatically...")

	// if fargate we strip the hostname
	if ecs.IsFargateInstance() {
		cache.Cache.Set(cacheHostnameKey, "", cache.NoExpiration)
		return "", nil
	}

	// GCE metadata
	log.Debug("GetHostname trying GCE metadata...")
	if getGCEHostname, found := hostname.ProviderCatalog["gce"]; found {
		name, err = getGCEHostname(name)
		if err == nil {
			cache.Cache.Set(cacheHostnameKey, name, cache.NoExpiration)
			return name, err
		}
		log.Debug("Unable to get hostname from GCE: ", err)
	}

	isContainerized, name := getContainerHostname()
	if isContainerized && name != "" {
		hostName = name
	}

	if hostName == "" {
		// os
		log.Debug("GetHostname trying os...")
		name, err = os.Hostname()
		if err == nil {
			hostName = name
		} else {
			log.Debug("Unable to get hostname from OS: ", err)
		}
	}

	/* at this point we've either the hostname from the os or an empty string */

	// We use the instance id if we're on an ECS cluster or we're on EC2
	// and the hostname is one of the default ones
	if getEC2Hostname, found := hostname.ProviderCatalog["ec2"]; found {
		log.Debug("GetHostname trying EC2 metadata...")
		instanceID, err := getEC2Hostname(name)
		if err == nil {
			err = ValidHostname(instanceID)
			if err == nil {
				hostName = instanceID
			} else {
				log.Debug("EC2 instance ID is not a valid hostname: ", err)
			}
		} else {
			log.Debug("Unable to determine hostname from EC2: ", err)
		}
	}

	// If at this point we don't have a name, bail out
	if hostName == "" {
		err = fmt.Errorf("unable to reliably determine the host name. You can define one in the agent config file or in your hosts file")
	} else {
		// we got a hostname, residual errors are irrelevant now
		err = nil
	}

	cache.Cache.Set(cacheHostnameKey, hostName, cache.NoExpiration)
	return hostName, err
}
