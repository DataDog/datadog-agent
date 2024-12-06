// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
)

func TestTomcatDefaultContextRootFromFile(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{
			filename: "foo.war",
			expected: "foo",
		},
		{
			filename: "foo",
			expected: "foo",
		},
		{
			filename: "foo#bar.war",
			expected: "foo/bar",
		},
		{
			filename: "ROOT.war",
			expected: "",
		},
		{
			filename: "foo##10.war",
			expected: "foo",
		},
		{
			filename: "foo#bar##15",
			expected: "foo/bar",
		},
		{
			filename: "ROOT##666",
			expected: "",
		},
	}
	extractor := newTomcatExtractor(NewDetectionContext(nil, envs.NewVariables(nil), nil))
	for _, tt := range tests {
		t.Run("Should parse "+tt.filename, func(t *testing.T) {
			value, ok := extractor.defaultContextRootFromFile(tt.filename)
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestScanDirForDeployments(t *testing.T) {
	//create our mock fs
	memfs := fstest.MapFS{
		"webapps/app1.war": &fstest.MapFile{},
		"webapps/app1":     &fstest.MapFile{Mode: fs.ModeDir},
		"webapps/app2":     &fstest.MapFile{Mode: fs.ModeDir},
		"webapps/app3":     &fstest.MapFile{Mode: fs.ModeDir},
	}
	tests := []struct {
		name     string
		path     string
		expected []jeeDeployment
	}{
		{
			name:     "dir not exist",
			path:     "nowhere",
			expected: nil,
		},
		{
			name: "should dedupe deployments",
			path: "webapps",
			expected: []jeeDeployment{
				{
					path: "webapps",
					name: "app1",
					dt:   war,
				},
				{
					path: "webapps",
					name: "app2",
					dt:   war,
				},
			},
		},
	}
	extractor := tomcatExtractor{ctx: NewDetectionContext(nil, envs.NewVariables(nil), memfs)}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployments := extractor.scanDirForDeployments(tt.path, &map[string]struct{}{
				"app3": {},
			})
			require.Equal(t, tt.expected, deployments)
		})
	}
}

func TestFindDeployedApps(t *testing.T) {

	tests := []struct {
		name       string
		domainHome string
		fs         fs.SubFS
		expected   []jeeDeployment
	}{
		{
			name: "tomcat - two virtual hosts",
			fs: fstest.MapFS{
				"webapps1/app1.war": &fstest.MapFile{},
				"webapps1/app1":     &fstest.MapFile{Mode: fs.ModeDir},
				"webapps1/app2":     &fstest.MapFile{Mode: fs.ModeDir},
				"webapps2/app2":     &fstest.MapFile{Mode: fs.ModeDir},
				"conf/server.xml": &fstest.MapFile{
					Data: []byte(`<Server port="8005" shutdown="SHUTDOWN">
  <Service name="Catalina">
    <Engine name="Catalina" defaultHost="host1">
		<Host name="host1"  appBase="webapps1"
				unpackWARs="true" autoDeploy="false">
			<Context docBase="app1" path="/context_1"/>
		</Host>
		<Host name="host2"  appBase="webapps2"
				unpackWARs="true" autoDeploy="false">
			<Context docBase="app2" path="/context_2"/>
		</Host>
	</Engine>
  </Service>
</Server>`),
				},
			},
			domainHome: ".",
			expected: []jeeDeployment{
				{
					name:        "app1",
					path:        "webapps1",
					dt:          war,
					contextRoot: "/context_1",
				},
				{
					name: "app2",
					path: "webapps1",
					dt:   war,
				},
				{
					name:        "app2",
					path:        "webapps2",
					dt:          war,
					contextRoot: "/context_2",
				},
			},
		},
		{
			name: "missing configuration",
			fs:   fstest.MapFS{},
		},
		{
			name: "malformed server configuration",
			fs: fstest.MapFS{
				"conf/server.xml": &fstest.MapFile{Data: []byte("bad")},
			},
		},
	}

	for _, tt := range tests {
		extractor := tomcatExtractor{ctx: NewDetectionContext(nil, envs.NewVariables(nil), tt.fs)}
		deployments, ok := extractor.findDeployedApps(tt.domainHome)
		require.Equal(t, len(tt.expected) > 0, ok)
		require.Equal(t, tt.expected, deployments)
	}
}
