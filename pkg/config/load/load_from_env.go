package load

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/conf"
)

func LoadProxyFromEnv(config conf.Config) {
	// Viper doesn't handle mixing nested variables from files and set
	// manually.  If we manually set one of the sub value for "proxy" all
	// other values from the conf file will be shadowed when using
	// 'config.Get("proxy")'. For that reason we first get the value from
	// the conf files, overwrite them with the env variables and reset
	// everything.

	// When FIPS proxy is enabled we ignore proxy setting to force data to the local proxy
	if config.GetBool("fips.enabled") {
		log.Infof("'fips.enabled' has been set to true. Ignoring proxy setting.")
		return
	}

	lookupEnvCaseInsensitive := func(key string) (string, bool) {
		value, found := os.LookupEnv(key)
		if !found {
			value, found = os.LookupEnv(strings.ToLower(key))
		}
		if found {
			log.Infof("Found '%v' env var, using it for the Agent proxy settings", key)
		}
		return value, found
	}

	lookupEnv := func(key string) (string, bool) {
		value, found := os.LookupEnv(key)
		if found {
			log.Infof("Found '%v' env var, using it for the Agent proxy settings", key)
		}
		return value, found
	}

	var isSet bool
	p := &conf.Proxy{}
	if isSet = config.IsSet("proxy"); isSet {
		if err := config.UnmarshalKey("proxy", p); err != nil {
			isSet = false
			log.Errorf("Could not load proxy setting from the configuration (ignoring): %s", err)
		}
	}

	if HTTP, found := lookupEnv("DD_PROXY_HTTP"); found {
		isSet = true
		p.HTTP = HTTP
	} else if HTTP, found := lookupEnvCaseInsensitive("HTTP_PROXY"); found {
		isSet = true
		p.HTTP = HTTP
	}

	if HTTPS, found := lookupEnv("DD_PROXY_HTTPS"); found {
		isSet = true
		p.HTTPS = HTTPS
	} else if HTTPS, found := lookupEnvCaseInsensitive("HTTPS_PROXY"); found {
		isSet = true
		p.HTTPS = HTTPS
	}

	if noProxy, found := lookupEnv("DD_PROXY_NO_PROXY"); found {
		isSet = true
		p.NoProxy = strings.Split(noProxy, " ") // space-separated list, consistent with viper
	} else if noProxy, found := lookupEnvCaseInsensitive("NO_PROXY"); found {
		isSet = true
		p.NoProxy = strings.Split(noProxy, ",") // comma-separated list, consistent with other tools that use the NO_PROXY env var
	}

	if !config.GetBool("use_proxy_for_cloud_metadata") {
		log.Debugf("'use_proxy_for_cloud_metadata' is enabled: adding cloud provider URL to the no_proxy list")
		isSet = true
		p.NoProxy = append(p.NoProxy,
			"169.254.169.254", // Azure, EC2, GCE
			"100.100.100.200", // Alibaba
		)
	}

	// We have to set each value individually so both config.Get("proxy")
	// and config.Get("proxy.http") work
	if isSet {
		config.Set("proxy.http", p.HTTP)
		config.Set("proxy.https", p.HTTPS)

		// If this is set to an empty []string, viper will have a type conflict when merging
		// this config during secrets resolution. It unmarshals empty yaml lists to type
		// []interface{}, which will then conflict with type []string and fail to merge.
		noProxy := make([]interface{}, len(p.NoProxy))
		for idx := range p.NoProxy {
			noProxy[idx] = p.NoProxy[idx]
		}
		config.Set("proxy.no_proxy", noProxy)
	}
}
