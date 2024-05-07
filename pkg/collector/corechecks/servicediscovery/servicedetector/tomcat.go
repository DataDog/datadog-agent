// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicedetector

import (
	"encoding/xml"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tomcat vendor specific constants
const (
	serverXmlPath = "conf/server.xml"
	rootWebApp    = "ROOT"
)

type (
	tomcatExtractor struct {
		ctx         ServiceDetector
		fileMatcher regexp.Regexp
	}

	tomcatServerXml struct {
		XmlName  xml.Name        `xml:"Server"`
		Services []tomcatService `xml:"Service"`
	}

	tomcatService struct {
		Hosts []tomcatHost `xml:"Engine>Host"`
	}

	tomcatHost struct {
		AppBase  string          `xml:"appBase,attr"`
		Contexts []tomcatContext `xml:"Context"`
	}

	tomcatContext struct {
		DocBase string `xml:"docBase,attr"`
		Path    string `xml:"path,attr"`
	}
)

func newTomcatExtractor(ctx ServiceDetector) vendorExtractor {
	return &tomcatExtractor{ctx: ctx}
}

// findDeployedApps looks for deployed application in the provided domainHome.
func (te tomcatExtractor) findDeployedApps(domainHome string) ([]jeeDeployment, bool) {
	serverXml := te.parseServerXml(domainHome)
	if serverXml == nil {
		return nil, false
	}
	var deployments []jeeDeployment
	uniques := make(map[string]struct{})
	for _, service := range serverXml.Services {
		for _, host := range service.Hosts {
			appBase := abs(host.AppBase, domainHome)
			for _, context := range host.Contexts {
				if context.DocBase != "" && context.Path != "" {
					deployment := tomcatCreateDeploymentFromFilePath(abs(context.DocBase, appBase))
					if _, ok := uniques[deployment.contextRoot]; !ok {
						uniques[deployment.name] = struct{}{}
						deployment.contextRoot = context.Path
						deployments = append(deployments, deployment)
					}
				}
			}
			// enrich with applications not having
			deployments = append(deployments, te.scanDirForDeployments(appBase, &uniques)...)
		}
	}
	return deployments, len(deployments) > 0
}

func (te tomcatExtractor) scanDirForDeployments(path string, uniques *map[string]struct{}) []jeeDeployment {
	entries, err := fs.ReadDir(te.ctx.fs, path)
	if err != nil {
		log.Debugf("error while scanning tomcat deployments (appBase: %s): %v", path, err)
		return nil
	}
	var ret []jeeDeployment
	// we can have both war and exploded deployments for the same deployment. So we have to dedupe
	for _, de := range entries {
		deployment := tomcatCreateDeploymentFromFilePath(de.Name())
		if _, ok := (*uniques)[deployment.name]; !ok {
			deployment.path = path
			(*uniques)[deployment.name] = struct{}{}
			ret = append(ret, deployment)
		}
	}
	return ret
}

func tomcatCreateDeploymentFromFilePath(fp string) jeeDeployment {
	d, f := path.Split(fp)
	stripped := strings.TrimSuffix(f, path.Ext(f))
	return jeeDeployment{
		path: path.Clean(d),
		name: stripped,
		dt:   war,
	}
}

func (tomcatExtractor) customExtractWarContextRoot(_ fs.FS) (string, bool) {
	// not supported
	return "", false
}

func (tomcatExtractor) defaultContextRootFromFile(fileName string) (string, bool) {
	keep, _, ok := strings.Cut(fileName, "##")
	if !ok {
		if i := strings.LastIndex(keep, "."); i >= 0 {
			keep = keep[:i]
		}
	}
	if keep == rootWebApp {
		return "", false
	}

	return strings.ReplaceAll(keep, "#", "/"), true
}

func (te tomcatExtractor) parseServerXml(domainHome string) *tomcatServerXml {
	xmlFilePath := path.Join(domainHome, serverXmlPath)
	file, err := te.ctx.fs.Open(xmlFilePath)
	if err != nil {
		log.Debugf("failed to locate tomcat server.xml (filepath: %s): %v", xmlFilePath, err)
		return nil
	}
	var serverXml tomcatServerXml
	err = xml.NewDecoder(file).Decode(&serverXml)
	if err != nil {
		log.Debugf("failed to parse tomcat server.xml(filepath: %s): %v", xmlFilePath, err)
		return nil
	}
	return &serverXml
}
