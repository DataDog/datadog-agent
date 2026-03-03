// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// SetSystemProxy configures the Windows system proxy for both WinINET (per-user) and WinHTTP.
// proxyURL should be a full URL like http://host:port
func SetSystemProxy(host *components.RemoteHost, proxyURL string) error {
	// Configure WinINET for the current user (used by many APIs and tools)
	ps := fmt.Sprintf(`
		$proxy = '%s'
		$path  = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
		Set-ItemProperty -Path $path -Name ProxyEnable -Value 1 -Type DWord
		Set-ItemProperty -Path $path -Name ProxyServer -Value $proxy -Type String
		# Configure WinHTTP for services and system components
		$u   = [uri]$proxy
		$hp  = $u.Host + ':' + $u.Port
		netsh winhttp set proxy proxy-server="http=$hp;https=$hp"
	`, proxyURL)
	if _, err := host.Execute(ps); err != nil {
		return err
	}
	return nil
}

// ResetSystemProxy disables the WinINET proxy for the current user and resets WinHTTP proxy.
func ResetSystemProxy(host *components.RemoteHost) error {
	ps := `
		$path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
		Set-ItemProperty -Path $path -Name ProxyEnable -Value 0 -Type DWord
		Remove-ItemProperty -Path $path -Name ProxyServer -ErrorAction SilentlyContinue
		netsh winhttp reset proxy
	`
	if _, err := host.Execute(ps); err != nil {
		return err
	}
	return nil
}

// BlockAllOutboundExceptProxy configures Windows Firewall to block all outbound traffic
// except to the specified proxy IP and port.
func BlockAllOutboundExceptProxy(host *components.RemoteHost, proxyIP string, port int) error {
	script := fmt.Sprintf(`
        $proxyIp = '%s'
        $proxyPort = %d
        Set-NetFirewallProfile -Profile Domain,Public,Private -DefaultOutboundAction Block
        $ruleName = "AllowProxy$proxyPort"
        if (-not (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue)) {
            New-NetFirewallRule -DisplayName $ruleName -Direction Outbound -Action Allow -RemoteAddress $proxyIp -Protocol TCP -RemotePort $proxyPort | Out-Null
        }
    `, proxyIP, port)
	_, err := host.Execute(script)
	if err != nil {
		return err
	}

	// Ensure outbound is blocked (a generic external call should fail)
	_, err = host.Execute(`curl.exe https://google.com`)
	if err == nil {
		return errors.New("outbound is not blocked")
	}

	return nil
}

// ResetOutboundPolicyAndRemoveProxyRules restores default outbound Allow and removes proxy allow rules.
func ResetOutboundPolicyAndRemoveProxyRules(host *components.RemoteHost) error {
	_, err := host.Execute(`
		$rules = Get-NetFirewallRule -DisplayName 'AllowProxy*' -ErrorAction SilentlyContinue
		if ($rules) { $rules | Remove-NetFirewallRule -ErrorAction SilentlyContinue }
		Set-NetFirewallProfile -Profile Domain,Public,Private -DefaultOutboundAction Allow
	`)
	if err != nil {
		return err
	}

	// Ensure outbound is allowed
	_, err = host.Execute(`curl.exe https://google.com`)
	if err != nil {
		return errors.New("outbound is not allowed")
	}

	return nil
}
