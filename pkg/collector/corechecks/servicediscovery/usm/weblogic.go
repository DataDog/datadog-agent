// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"encoding/xml"
	"io/fs"
	"path"

	"go.uber.org/zap"
)

// weblogic vendor specific constants
const (
	wlsServerNameSysProp string = "-Dweblogic.Name="
	wlsServerConfigFile  string = "config.xml"
	wlsServerConfigDir   string = "config"
	weblogicXMLFile      string = "META-INF/weblogic.xml"
)

type (
	weblogicExtractor struct {
		ctx DetectionContext
	}
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

func newWeblogicExtractor(ctx DetectionContext) vendorExtractor {
	return &weblogicExtractor{ctx: ctx}
}

// findDeployedApps looks for deployed application in the provided domainHome.
// The args is required here because used to determine the current server name.
// it returns paths for staged only applications and bool being true if at least one application is found
func (we weblogicExtractor) findDeployedApps(domainHome string) ([]jeeDeployment, bool) {
	serverName, ok := extractJavaPropertyFromArgs(we.ctx.args, wlsServerNameSysProp)
	if !ok {
		return nil, false
	}
	serverConfigFile, err := we.ctx.fs.Open(path.Join(domainHome, wlsServerConfigDir, wlsServerConfigFile))
	if err != nil {
		we.ctx.logger.Debug("weblogic: unable to open config.xml", zap.Error(err))
		return nil, false
	}
	defer serverConfigFile.Close()
	if ok, err := canSafelyParse(serverConfigFile); !ok {
		we.ctx.logger.Debug("weblogic: config.xml looks too big", zap.Error(err))
		return nil, false
	}
	var deployInfos weblogicDeploymentInfo
	err = xml.NewDecoder(serverConfigFile).Decode(&deployInfos)

	if err != nil {
		we.ctx.logger.Debug("weblogic: cannot parse config.xml", zap.Error(err))
		return nil, false
	}
	var deployments []jeeDeployment
	for _, di := range deployInfos.AppDeployment {
		if di.StagingMode == "stage" && di.Target == serverName {
			_, name := path.Split(di.SourcePath)
			deployments = append(deployments, jeeDeployment{name: name, path: di.SourcePath})
		}
	}
	return deployments, len(deployments) > 0
}

func (weblogicExtractor) customExtractWarContextRoot(warFS fs.FS) (string, bool) {
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

func (weblogicExtractor) defaultContextRootFromFile(fileName string) (string, bool) {
	return standardExtractContextFromWarName(fileName)
}
