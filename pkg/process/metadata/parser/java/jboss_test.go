// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestJbossExtractServerName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name: "server name present",
			args: []string{
				"java",
				"-D[Server:server1]",
			},
			expected: "server1",
		},
		{
			name: "server name absent",
			args: []string{
				"java",
				"-D[Standalone]",
			},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := jbossExtractServerName(tt.args)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestJbossExtractConfigFileName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		domain   bool
		expected string
	}{
		{
			name:     "default for standalone",
			args:     nil,
			domain:   false,
			expected: "standalone.xml",
		},
		{
			name:     "standalone with short option",
			args:     []string{"java", "-jar", "-jboss-modules.jar", "-c", "standalone-ha.xml"},
			domain:   false,
			expected: "standalone-ha.xml",
		},
		{
			name:     "standalone with long option",
			args:     []string{"java", "-jar", "-jboss-modules.jar", "--server-config=standalone-full.xml"},
			domain:   false,
			expected: "standalone-full.xml",
		},
		{
			name:     "default for domain",
			args:     nil,
			domain:   true,
			expected: "domain.xml",
		},
		{
			name:     "domain with short option",
			args:     []string{"java", "-jar", "-jboss-modules.jar", "-c", "domain-ha.xml"},
			domain:   true,
			expected: "domain-ha.xml",
		},
		{
			name:     "standalone with long option",
			args:     []string{"java", "-jar", "-jboss-modules.jar", "--domain-config=domain-full.xml"},
			domain:   true,
			expected: "domain-full.xml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, jbossExtractConfigFileName(tt.args, tt.domain))
		})
	}
}

func TestJbossFindServerGroup(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		hostXML    string
		expected   string
	}{
		{
			name:       "server group found",
			serverName: "server-two",
			hostXML: `<?xml version="1.0" encoding="UTF-8"?>
<host xmlns="urn:jboss:domain:20.0" name="primary">
    <servers>
        <server name="server-one" group="main-server-group"/>
        <server name="server-two" group="main-server-group" auto-start="true">
            <jvm name="default"/>
            <socket-bindings port-offset="150"/>
        </server>
        <server name="server-three" group="other-server-group" auto-start="false">
            <jvm name="default"/>
            <socket-bindings port-offset="250"/>
        </server>
    </servers>
</host>`,
			expected: "main-server-group",
		},
		{
			name:       "server group not found",
			serverName: "server-four",
			hostXML: `<?xml version="1.0" encoding="UTF-8"?>
<host xmlns="urn:jboss:domain:20.0" name="primary">
    <servers>
        <server name="server-one" group="main-server-group"/>
        <server name="server-two" group="main-server-group" auto-start="true">
            <jvm name="default"/>
            <socket-bindings port-offset="150"/>
        </server>
        <server name="server-three" group="other-server-group" auto-start="false">
            <jvm name="default"/>
            <socket-bindings port-offset="250"/>
        </server>
    </servers>
</host>`,
			expected: "",
		},
		{
			name:       "empty host.xml",
			serverName: "server-one",
			hostXML:    "",
			expected:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			if len(tt.hostXML) > 0 {
				require.NoError(t, afero.WriteFile(memFs, "/configuration/host.xml", []byte(tt.hostXML), 0664))
			}
			value, ok := jbossFindServerGroup(afero.NewBasePathFs(memFs, "/"), tt.serverName)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestJbossExtractWarContextRoot(t *testing.T) {
	tests := []struct {
		name        string
		jbossWebXML string
		location    string
		expected    string
	}{
		{
			name: "jboss-web in META-INF",
			jbossWebXML: `<?xml version="1.0" encoding="UTF-8"?>
<jboss-web version="7.1" xmlns="http://www.jboss.com/xml/ns/javaee" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.jboss.com/xml/ns/javaee http://www.jboss.org/schema/jbossas/jboss-web_7_1.xsd">
    <context-root>/myapp</context-root>
</jboss-web>`,
			location: "/META-INF/jboss-web.xml",
			expected: "/myapp",
		},
		{
			name:        "jboss-web in WEB-INF",
			jbossWebXML: "<jboss-web><context-root>/yourapp</context-root></jboss-web>",
			location:    "/WEB-INF/jboss-web.xml",
			expected:    "/yourapp",
		},
		{
			name:        "jboss-web in WEB-INF without context-root",
			jbossWebXML: "<jboss-web/>",
			location:    "/WEB-INF/jboss-web.xml",
			expected:    "",
		},
		{
			name:     "jboss-web missing",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			if len(tt.location) > 0 {
				require.NoError(t, afero.WriteFile(memFs, tt.location, []byte(tt.jbossWebXML), 0664))
			}
			value, ok := jbossExtractWarContextRoot(afero.NewBasePathFs(memFs, "/"))
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestJbossFindDeployedApps(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		domainHome string
		expected   []jeeDeployment
	}{
		{
			name: "standalone",
			args: []string{
				"-Djboss.home.dir=testdata/jboss",
			},
			domainHome: "testdata/jboss/standalone",
			expected: []jeeDeployment{
				{
					name: "app.ear",
					path: filepath.FromSlash("testdata/jboss/standalone/data/content/38/e/content"),
				},
				{
					name: "web3.war",
					path: filepath.FromSlash("testdata/jboss/standalone/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content"),
				},
				{
					name: "web4.war",
					path: filepath.FromSlash("testdata/jboss/standalone/data/content/f0/c/content"),
				},
			},
		},
		{
			name: "domain - main server group",
			args: []string{
				"-Djboss.home.dir=testdata/jboss",
				"-D[Server:server-one]",
			},
			domainHome: "testdata/jboss/domain/servers/server-one",
			expected: []jeeDeployment{
				{
					name: "app.ear",
					path: filepath.FromSlash("testdata/jboss/domain/servers/server-one/data/content/38/e/content"),
				},
				{
					name: "web3.war",
					path: filepath.FromSlash("testdata/jboss/domain/servers/server-one/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content"),
				},
				{
					name: "web4.war",
					path: filepath.FromSlash("testdata/jboss/domain/servers/server-one/data/content/f0/c/content"),
				},
			},
		},
		{
			name: "domain- other server group",
			args: []string{
				"-Djboss.home.dir=testdata/jboss",
				"-D[Server:server-three]",
			},
			domainHome: "testdata/jboss/domain/servers/server-three",
			expected: []jeeDeployment{
				{
					name: "web4.war",
					path: filepath.FromSlash("testdata/jboss/domain/servers/server-three/data/content/f0/c/content"),
				},
			},
		},
		{
			name: "domain- server not found",
			args: []string{
				"-Djboss.home.dir=testdata/jboss",
				"-D[Server:server-four]",
			},
			domainHome: "testdata/jboss/domain/servers/server-four",
			expected:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := jbossFindDeployedApps(filepath.FromSlash(tt.domainHome), tt.args, afero.NewOsFs())
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}
