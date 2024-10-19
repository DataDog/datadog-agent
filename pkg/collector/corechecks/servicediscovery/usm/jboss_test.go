// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package usm

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
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
		name          string
		serverName    string
		hostXML       string
		expected      string
		errorExpected bool
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
			name:          "empty host.xml",
			serverName:    "server-one",
			hostXML:       "",
			expected:      "",
			errorExpected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memFs := fstest.MapFS{}
			if len(tt.hostXML) > 0 {
				memFs["configuration/host.xml"] = &fstest.MapFile{Data: []byte(tt.hostXML)}
			}
			value, ok, err := jbossFindServerGroup(memFs, tt.serverName)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
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
			location: "META-INF/jboss-web.xml",
			expected: "/myapp",
		},
		{
			name:        "jboss-web in WEB-INF",
			jbossWebXML: "<jboss-web><context-root>/yourapp</context-root></jboss-web>",
			location:    "WEB-INF/jboss-web.xml",
			expected:    "/yourapp",
		},
		{
			name:        "jboss-web in WEB-INF without context-root",
			jbossWebXML: "<jboss-web/>",
			location:    "WEB-INF/jboss-web.xml",
			expected:    "",
		},
		{
			name:     "jboss-web missing",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memFs := fstest.MapFS{}
			if len(tt.location) > 0 {
				memFs[tt.location] = &fstest.MapFile{Data: []byte(tt.jbossWebXML)}
			}
			value, ok := newJbossExtractor(NewDetectionContext(nil, envs.NewVariables(nil), nil)).customExtractWarContextRoot(memFs)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestJbossFindDeployedApps(t *testing.T) {
	sub := MakeTestSubDirFS(t)
	tests := []struct {
		name       string
		args       []string
		domainHome string
		expected   []jeeDeployment
		fs         fs.SubFS
	}{
		{
			name: "standalone",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
			},
			domainHome: jbossTestAppRootAbsolute + "/standalone",
			expected: []jeeDeployment{
				{
					name: "app.ear",
					path: jbossTestAppRootAbsolute + "/standalone/data/content/38/e/content",
				},
				{
					name: "web3.war",
					path: jbossTestAppRootAbsolute + "/standalone/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content",
				},
				{
					name: "web4.war",
					path: jbossTestAppRootAbsolute + "/standalone/data/content/f0/c/content",
				},
			},
			fs: sub,
		},
		{
			name: "standalone - missing home",
			args: nil,
			fs:   RealFs{},
		},
		{
			name: "standalone - missing config",
			args: []string{
				"-Djboss.home.dir=jboss",
			},
			fs: fstest.MapFS{
				"jboss/standalone/configuration": &fstest.MapFile{Mode: fs.ModeDir},
			},
		},
		{
			name: "standalone - bad config",
			args: []string{
				"-Djboss.home.dir=jboss",
			},
			fs: fstest.MapFS{
				"jboss/standalone/configuration/standalone.xml": &fstest.MapFile{Data: []byte("evil")},
			},
		},
		{
			name: "domain - main server group",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-one]",
			},
			domainHome: "" + jbossTestAppRoot + "/domain/servers/server-one",
			expected: []jeeDeployment{
				{
					name: "app.ear",
					path: "" + jbossTestAppRoot + "/domain/servers/server-one/data/content/38/e/content",
				},
				{
					name: "web3.war",
					path: "" + jbossTestAppRoot + "/domain/servers/server-one/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content",
				},
				{
					name: "web4.war",
					path: "" + jbossTestAppRoot + "/domain/servers/server-one/data/content/f0/c/content",
				},
			},
			fs: sub,
		},
		{
			name: "domain- other server group",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-three]",
			},
			domainHome: "" + jbossTestAppRoot + "/domain/servers/server-three",
			expected: []jeeDeployment{
				{
					name: "web4.war",
					path: "" + jbossTestAppRoot + "/domain/servers/server-three/data/content/f0/c/content",
				},
			},
			fs: sub,
		},
		{
			name: "domain- server not found",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-four]",
			},
			domainHome: "" + jbossTestAppRoot + "/domain/servers/server-four",
			expected:   nil,
			fs:         sub,
		},
		{
			name: "domain- malformed server",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:]",
			},
			domainHome: "" + jbossTestAppRoot + "/domain/servers/server-four",
			expected:   nil,
			fs:         sub,
		},
		{
			name: "domain- missing dir",
			args: []string{
				"-Djboss.home.dir=jboss",
				"-D[Server:server-one]",
			},
			expected: nil,
			fs: fstest.MapFS{
				"jboss/configuration": &fstest.MapFile{Mode: fs.ModeDir},
			},
		},
		{
			name: "domain- missing domain.xml",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-one]",
			},
			expected: nil,
			fs: shadowFS{
				filesystem: RealFs{},
				globs:      []string{"" + jbossTestAppRoot + "/domain/configuration/domain.xml"},
			},
		},
		{
			name: "domain- missing files",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-one]",
			},
			expected: nil,
			fs: shadowFS{
				filesystem: RealFs{},
				globs:      []string{"" + jbossTestAppRoot + "/domain/configuration/host.xml"},
			},
		},
		{
			name: "domain- broken domain.xml",
			args: []string{
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-D[Server:server-one]",
			},
			domainHome: "" + jbossTestAppRoot,
			expected:   nil,
			fs: chainedFS{
				chain: []fs.FS{
					fstest.MapFS{
						"" + jbossTestAppRoot + "/domain/configuration/domain.xml": &fstest.MapFile{Data: []byte("evil")},
					},
					RealFs{},
				},
			},
		},
		{
			name: "domain- broken host.xml",
			args: []string{
				"-Djboss.home.dir=opt/jboss",
				"-D[Server:server-one]",
			},
			domainHome: "opt/jboss/domain/servers/server-one",
			expected:   nil,
			fs: fstest.MapFS{
				"opt/jboss/domain/server/domain/configuration/host.xml": &fstest.MapFile{Data: []byte("broken")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A sibling directory is used as PWD since the tests uses relative
			// paths to the app root (../testdata/a).
			envsMap := map[string]string{
				"PWD": "/sibling",
			}
			value, ok := newJbossExtractor(NewDetectionContext(tt.args, envs.NewVariables(envsMap), tt.fs)).findDeployedApps(tt.domainHome)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}
