// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package util

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
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
	hostname = strings.TrimSpace(hostname)
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

// GetHostname retrieve the host name for the Agent, trying to query
// compiled-in providers. Result is cached
func GetHostname() (string, error) {
	cacheHostnameKey := cache.BuildAgentKey("hostname")
	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		return cacheHostname.(string), nil
	}

	name, err := lookupHostname()
	if err != nil {
		return "", err
	}
	cache.Cache.Set(cacheHostnameKey, name, cache.NoExpiration)
	return name, nil
}

func lookupHostname() (string, error) {
	// try the name provided in the configuration file
	name := config.Datadog.GetString("hostname")
	err := ValidHostname(name)
	if err == nil {
		return name, err
	}

	log.Infof("unable to get the hostname from the config file: %s", err)
	log.Info("trying to determine a reliable host name automatically...")

	for _, providers := range hostname.ProviderCatalog {
		for _, provider := range providers {
			log.Debugf("trying to get hostname from %s...", provider.Name)
			name, err := provider.Method("")
			if err != nil {
				log.Debugf("cannot get hostname from %s: %s", provider.Name, err)
				continue
			}
			err = ValidHostname(name)
			if err != nil {
				log.Debugf("invalid hostname from %s: %s", provider.Name, err)
				continue
			}
			log.Infof("got hostname \"%s\" from %s", name, provider.Name)
			return name, nil
		}
	}

	// If at this point we don't have a name, bail out
	return "", fmt.Errorf("Unable to reliably determine the host name. You can define one in the agent config file or in your hosts file")
}
