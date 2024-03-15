// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/afero/zipfs"
	"github.com/stretchr/testify/require"
)

// TestWeblogicFindDeployedApps tests the ability to extract deployed application from a weblogic config.xml
// The file contains staged and non-staged deployments for different servers.
// It is expected that only the staged deployment of `AdminServer` are returned.
func TestWeblogicFindDeployedApps(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		expected   []jeeDeployment
	}{
		{
			name:       "multiple deployments for multiple server - extract for AdminServer",
			serverName: "AdminServer",
			expected: []jeeDeployment{
				{
					name: "test.war",
					path: "java/testdata/weblogic/test.war",
				},
				{
					name: "sample4.war",
					path: "/u01/oracle/user_projects/tmp/sample4.war",
				},
				{
					name: "test.ear",
					path: "java/testdata/weblogic/test.ear",
				},
			},
		},
		{
			name:     "server name is missing",
			expected: nil,
		},
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	domainHome := filepath.Join(cwd, "testdata", "weblogic")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			if len(tt.serverName) > 0 {
				args = append(args, wlsServerNameSysProp+tt.serverName)
			}
			value, ok := weblogicFindDeployedApps(domainHome, args, afero.NewOsFs())
			require.Equal(t, len(value) > 0, ok)
			require.Equal(t, tt.expected, value)
		})
	}
}

func TestWeblogicExtractWarContextRoot(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		xmlContent string
		expected   string
	}{
		{
			name: "war with weblogic.xml and context-root",
			xmlContent: `
<weblogic-web-app xmlns="http://xmlns.oracle.com/weblogic/weblogic-web-app" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.oracle.com/weblogic/weblogic-web-app
http://xmlns.oracle.com/weblogic/weblogic-web-app/1.4/weblogic-web-app.xsd">
<context-root>my-context</context-root>
</weblogic-web-app>`,
			expected: "my-context",
		},
		{
			name: "weblogic.xml without context-root",
			xmlContent: `
<weblogic-web-app xmlns="http://xmlns.oracle.com/weblogic/weblogic-web-app" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.oracle.com/weblogic/weblogic-web-app
http://xmlns.oracle.com/weblogic/weblogic-web-app/1.4/weblogic-web-app.xsd"/>`,
			expected: "",
		},
		{
			name:     "no weblogic.xml in the war",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create an in memory zip to emulate a war
			buf := bytes.NewBuffer([]byte{})
			writer := zip.NewWriter(buf)
			if len(tt.xmlContent) > 0 {
				require.NoError(t, writeFile(writer, weblogicXMLFile, tt.xmlContent))
			}
			require.NoError(t, writer.Close())

			// now create a zip reader to pass to the tested function
			reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)
			value, ok := weblogicExtractWarContextRoot(zipfs.New(reader))
			require.Equal(t, len(tt.expected) > 0, ok)
			require.Equal(t, tt.expected, value)
		})
	}
}

// TestWeblogicExtractExplodedWarContextRoot tests the ability to extract context root from weblogic.xml
// when the deployment is exploded (aka is a directory and not a war archive)
func TestWeblogicExtractExplodedWarContextRoot(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fs := afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(cwd, "testdata", "weblogic", "test.war"))
	value, ok := weblogicExtractWarContextRoot(fs)
	require.True(t, ok)
	require.Equal(t, "my_context", value)
}
