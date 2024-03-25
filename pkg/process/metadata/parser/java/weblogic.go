// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"encoding/xml"
	"path/filepath"

	"github.com/spf13/afero"
)

// weblogic vendor specific constants
const (
	wlsServerNameSysProp string = "-Dwls.Name="
	wlsServerConfigFile  string = "config.xml"
	wlsServerConfigDir   string = "config"
	weblogicXMLFile      string = "/META-INF/weblogic.xml"
)

type (
	// weblogicDeploymentInfo reflects the domain type of weblogic config.xml
	weblogicDeploymentInfo struct {
		XMLName       xml.Name                `xml:"domain"`
		AppDeployment []weblogicAppDeployment `xml:"app-deployment"`
	}

	// weblogicAppDeployment reflects a deployment information in config.xml
	weblogicAppDeployment struct {
		Target      string `xml:"target"`
		SourcePath  string `xml:"source-path"`
		StagingMode string `xml:"staging-mode"`
	}

	// weblogicXMLContextRoot allows to extract the context-root tag value from weblogic.xml on war archives
	weblogicXMLContextRoot struct {
		XMLName     xml.Name `xml:"weblogic-web-app"`
		ContextRoot string   `xml:"context-root"`
	}
)

// weblogicFindDeployedApps looks for deployed application in the provided domainHome.
// The args is required here because used to determine the current server name.
// it returns paths for staged only applications and bool being true if at least one application is found
func weblogicFindDeployedApps(domainHome string, args []string, fs afero.Fs) ([]typedDeployment, bool) {
	serverName, ok := extractJavaPropertyFromArgs(args, wlsServerNameSysProp)
	if !ok {
		return nil, false
	}
	serverConfigFile, err := fs.Open(filepath.Join(domainHome, wlsServerConfigDir, wlsServerConfigFile))
	if err != nil {
		return nil, false
	}
	defer serverConfigFile.Close()
	if !canSafelyParse(serverConfigFile) {
		return nil, false
	}
	var deployInfos weblogicDeploymentInfo
	err = xml.NewDecoder(serverConfigFile).Decode(&deployInfos)

	if err != nil {
		return nil, false
	}
	var deployments []typedDeployment
	for _, di := range deployInfos.AppDeployment {
		if di.StagingMode == "stage" && di.Target == serverName {
			deployments = append(deployments, typedDeployment{path: di.SourcePath})
		}
	}
	return deployments, len(deployments) > 0
}

func weblogicExtractWarContextRoot(warFS afero.Fs) (string, bool) {
	// vfs package will internally clean the filename to comply with the os separators
	file, err := warFS.Open(weblogicXMLFile)
	if err != nil {
		return "", false
	}
	defer file.Close()
	var wlsXML weblogicXMLContextRoot
	if xml.NewDecoder(file).Decode(&wlsXML) != nil || len(wlsXML.ContextRoot) == 0 {
		return "", false
	}
	return wlsXML.ContextRoot, true
}
