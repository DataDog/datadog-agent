// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo || !dll_hostname || serverless

package hostname

import "fmt"

func dllResolveHostname(providerName string, hostnameFile string) (string, error) {
	return "", fmt.Errorf("DLL hostname resolution is disabled")
}

// getDLLProviders returns no providers when DLL hostname is disabled.
func getDLLOSProvider() provider {
	return provider{
		name:             "dll_os_disabled",
		cb:               nil,
		stopIfSuccessful: false,
		expvarName:       "dll_os_disabled",
	}
}

// getDLLFQDNProvider returns no providers when DLL hostname is disabled.
func getDLLFQDNProvider() provider {
	return provider{
		name:             "dll_fqdn_disabled",
		cb:               nil,
		stopIfSuccessful: false,
		expvarName:       "dll_fqdn_disabled",
	}
}

// getDLLHostnameFileProvider returns no providers when DLL hostname is disabled.
func getDLLHostnameFileProvider() provider {
	return provider{
		name:             "dll_hostname_file_disabled",
		cb:               nil,
		stopIfSuccessful: true,
		expvarName:       "dll_hostname_file_disabled",
	}
}
