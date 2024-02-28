// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package javaparser contains functions to autodetect service name for java applications
package javaparser

import (
	"encoding/xml"
	"io"
	"strings"
)

// appserver is an enumeration of application server types
type serverVendor uint8

// appserver enums
const (
	unknown serverVendor = 0
	jboss                = 1 << (iota - 1)
	tomcat
	weblogic
	websphere
)

const (
	// app servers hints
	wlsServerMainClass  string = "weblogic.Server"
	wlsHomeSysProp      string = "-Dwls.home="
	websphereJar        string = "ws-server.jar"
	websphereMainClass  string = "defaultServer"
	tomcatMainClass     string = "org.apache.catalina.startup.Bootstrap"
	tomcatSysProp       string = "-Dcatalina.base="
	jbossStandaloneMain string = "org.jboss.as.standalone"
	jbossDomainMain     string = "org.jboss.as.server"
	jbossSysProp        string = "-Djboss.home.dir="
)

// applicationXml is used to unmarshal information from a standard EAR's application.xml
// example doc: https://docs.oracle.com/cd/E13222_01/wls/docs61/programming/app_xml.html
type applicationXml struct {
	XMLName     xml.Name `xml:"application"`
	ContextRoot []string `xml:"module>web>context-root"`
}

// extractContextRootFromApplicationXml parses a standard application.xml file extracting
// mount points for web application (aka context roots).
func extractContextRootFromApplicationXml(reader io.Reader) ([]string, error) {
	var a applicationXml
	err := xml.NewDecoder(reader).Decode(&a)
	if err != nil {
		return nil, err
	}
	return a.ContextRoot, nil
}

// resolveAppServerFromCmdLine parses the command line and tries to extract a couple of evidences for each known application server.
// The first is the server home (usually defined by a service
func resolveAppServerFromCmdLine(args []string) (serverVendor, string) {
	hint1, hint2 := unknown, unknown
	var baseDir string
	for _, a := range args {
		if hint1 == unknown {
			if strings.HasPrefix(a, wlsHomeSysProp) {
				hint1 = weblogic
				baseDir = strings.TrimPrefix(a, wlsHomeSysProp)
			} else if strings.HasPrefix(a, tomcatSysProp) {
				hint1 = tomcat
				baseDir = strings.TrimPrefix(a, tomcatSysProp)
			} else if strings.HasPrefix(a, jbossSysProp) {
				hint1 = jboss
				baseDir = strings.TrimPrefix(a, jbossSysProp)
			} else if strings.HasSuffix(a, websphereJar) {
				// Use the CWD of the process as websphere baseDir
				hint1 = websphere
			}
		}
		if hint2 == unknown {
			// only return a match if it's exact meaning that the hint and the evidence are matching the same server type.
			switch a {
			case wlsServerMainClass:
				hint2 = weblogic
			case tomcatMainClass:
				hint2 = tomcat
			case websphereMainClass:
				hint2 = websphere
			case jbossDomainMain, jbossStandaloneMain:
				hint2 = jboss
			}
		}
		if hint1 != unknown && hint2 != unknown {
			break
		}
	}
	return hint1 & hint2, baseDir
}
