// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func createProxyConnection(ip pulumi.StringInput, user string, sshKeyContent pulumi.StringOutput, proxyConn remote.ConnectionOutput) remote.ConnectionOutput {
	conn := remote.ConnectionArgs{
		Host:           ip,
		PerDialTimeout: pulumi.IntPtr(5),
		DialErrorLimit: pulumi.IntPtr(60),
		User:           pulumi.StringPtr(user),
		PrivateKey:     sshKeyContent,
	}

	conn.Proxy = remote.ProxyConnectionPtr(&remote.ProxyConnectionArgs{
		AgentSocketPath:    proxyConn.AgentSocketPath(),
		DialErrorLimit:     proxyConn.DialErrorLimit(),
		Host:               proxyConn.Host(),
		Password:           proxyConn.Password(),
		PerDialTimeout:     proxyConn.PerDialTimeout(),
		Port:               proxyConn.Port(),
		PrivateKey:         proxyConn.PrivateKey(),
		PrivateKeyPassword: proxyConn.PrivateKeyPassword(),
		User:               proxyConn.User(),
	})

	return conn.ToConnectionOutput()
}
