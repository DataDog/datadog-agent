package util

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetDockerHostIP returns the IP address of the host
func GetDockerHostIP() []string {
	cacheKey := cache.BuildAgentKey("hostIPs")
	if cachedIPs, found := cache.Cache.Get(cacheKey); found {
		return cachedIPs
	}

	ips := getDockerHostIPUncached()
	if len(ips) == 0 {
		log.Warnf("could not get host IP")
	}
	cache.Cache.Set(cacheKey, ips)
	return ips
}

func getDockerHostIPUncached() []string {
	var lastErr error

	for _, attempt := range []struct {
		name     string
		provider func() ([]string, error)
	}{
		{"config", getHostIPFFromConfig},
		{"ec2 metadata api", ec2.GetLocalIPv4},
	} {
		log.Debugf("attempting to get host ip from source: %s", attempt.name)
		ips, err = attempt.provider()
		if err != nil {
			lastErr = err
			log.Debugf("could not deduce host IP from source: %s", err)
		} else {
			return ips
		}
	}
	return nil
}

func getHostIPFFromConfig() ([]string, error) {
	hostIP := config.DataDog.GetString("process_agent_config.host_ip")
	if hostIP == "" {
		return nil, fmt.Errorf("no host IP configured")
	}
	return []string{hostIP}
}
