// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package javaparser contains functions to autodetect service name for java applications
package javaparser

import (
	"encoding/xml"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

const (
	jbossServerName               = "-D[Server:"
	jbossHomeDirSysProp           = "-Djboss.home.dir="
	jbossConfigDir                = "configuration"
	jbossHostXMLFile              = "host.xml"
	jbossDefaultStandaloneXMLFile = "standalone.xml"
	jbossDefaultDomainXMLFile     = "domain.xml"
	jbossDomainBase               = "domain"
	jbossStandaloneBase           = "standalone"
	jbossDataDir                  = "data"
	jbossContentDir               = "content"
	jbossServerConfigShort        = "-c"
	jbossServerConfig             = "--server-config"
	jbossDomainConfig             = "--domain-config"
	jbossWebXMLFileMetaInf        = "/META-INF/jboss-web.xml"
	jbossWebXMLFileWebInf         = "/WEB-INF/jboss-web.xml"
)

type (
	// jbossHostsXML allows unmarshalling the host.xml file
	jbossHostXML struct {
		XMLName xml.Name          `xml:"host"`
		Servers []jbossHostServer `xml:"servers>server"`
	}
	// jbossHostsServer contains the mapping between server name and server group
	jbossHostServer struct {
		Name  string `xml:"name,attr"`
		Group string `xml:"group,attr"`
	}
	// jbossStandaloneXML allows unmarshalling the standalone.xml file
	jbossStandaloneXML struct {
		XMLName     xml.Name                `xml:"server"`
		Deployments []jbossServerDeployment `xml:"deployments>deployment"`
	}
	// jbossServerDeployment contains information about deployed content on a jboss instance
	jbossServerDeployment struct {
		Name        string `xml:"name,attr"`
		RuntimeName string `xml:"runtime-name,attr"`
		// Enabled by default is true.
		Enabled string               `xml:"enabled,attr,"`
		Content jbossDeployedContent `xml:"content"`
	}
	// jbossDeployedContent hold the sha1 of a deployed content (will map to a dir)
	jbossDeployedContent struct {
		Hash string `xml:"sha1,attr"`
	}
	//jbossServerGroup contains information about the server groups defined for a host.
	jbossServerGroup struct {
		Name        string                  `xml:"name,attr"`
		Deployments []jbossServerDeployment `xml:"deployments>deployment"`
	}
	// jbossDomainXML allows unmarshalling the domain.xml file
	jbossDomainXML struct {
		XMLName      xml.Name                `xml:"domain"`
		Deployments  []jbossServerDeployment `xml:"deployments>deployment"`
		ServerGroups []jbossServerGroup      `xml:"server-groups>server-group"`
	}

	// jbossWebXML allows unmarshalling the jboss-web.xml file
	jbossWebXML struct {
		XMLName     xml.Name `xml:"jboss-web"`
		ContextRoot string   `xml:"context-root"`
	}
)

// jbossFindDeployedApps is the entry point function to find deployed application on a jboss instance
// It detects if the instance is standalone or part of a cluster (domain). It returns a slice of jeeDeployment and a bool.
// That will be false in case no deployments have been found
func jbossFindDeployedApps(domainHome string, args []string, fs afero.Fs) ([]jeeDeployment, bool) {
	baseDir, ok := extractJavaPropertyFromArgs(args, jbossHomeDirSysProp)
	if !ok {
		return nil, false
	}
	serverName, domainMode := jbossExtractServerName(args)
	configFile := jbossExtractConfigFileName(args, domainMode)
	var deployments []jbossServerDeployment
	if domainMode {
		baseDir = filepath.Join(baseDir, jbossDomainBase)
		deployments = jbossDomainFindDeployments(afero.NewBasePathFs(fs, baseDir), configFile, serverName)
	} else {
		baseDir = filepath.Join(baseDir, jbossStandaloneBase)
		deployments = jbossStandaloneFindDeployments(afero.NewBasePathFs(fs, baseDir), configFile)
	}
	if len(deployments) == 0 {
		return nil, false
	}
	ret := make([]jeeDeployment, 0, len(deployments))
	for _, d := range deployments {
		// applications are in /data/xx/yyyyy (xx are the first two chars of the sha1)
		ret = append(ret, jeeDeployment{name: d.RuntimeName,
			path: filepath.Join(domainHome, jbossDataDir, jbossContentDir, d.Content.Hash[:2], d.Content.Hash[2:], jbossContentDir),
		})
	}
	return ret, true
}

// jbossExtractWarContextRoot inspects a war deployment in order to find the jboss-web.xml file under META-INF or WEB-INF.
// If a context root is found it's returned otherwise the function will return "", false
func jbossExtractWarContextRoot(warFS afero.Fs) (string, bool) {
	file, err := warFS.Open(jbossWebXMLFileWebInf)
	if err != nil {
		// that file can be in WEB-INF or META-INF.
		file, err = warFS.Open(jbossWebXMLFileMetaInf)
		if err != nil {
			return "", false
		}

	}
	defer file.Close()
	var jwx jbossWebXML
	if xml.NewDecoder(file).Decode(&jwx) != nil || len(jwx.ContextRoot) == 0 {
		return "", false
	}
	return jwx.ContextRoot, true
}

// jbossExtractConfigFileName allows to parse from the args the right filename that's containing the server configuration.
// For standalone, it defaults to standalone.xml, for domain, to domain.xml
func jbossExtractConfigFileName(args []string, domain bool) string {
	// determine which long argument look for depending on the mode (standalone or domain)
	longArg := jbossServerConfig
	defaultConfig := jbossDefaultStandaloneXMLFile
	if domain {
		longArg = jbossDomainConfig
		defaultConfig = jbossDefaultDomainXMLFile
	}
	for i, a := range args {
		if !strings.HasPrefix(a, jbossServerConfigShort) && !strings.HasPrefix(a, longArg) {
			continue
		}
		// the argument can be declared either with space (i.e. `-c conf.xml`) either with = (i.e. `-c=conf.xml`)
		parts := strings.SplitN(a, "=", 2)
		// return the right part if separated by an equal
		if len(parts) > 1 {
			return parts[1]
		}
		// return the next arg if separated by a space
		if len(args) > i {
			return args[i+1]
		}
		// cannot parse return an empty filename
		return ""
	}
	return defaultConfig
}

// jbossDomainFindDeployments finds active deployments for a server in domain mode.
func jbossDomainFindDeployments(basePathFs afero.Fs, configFile string, serverName string) []jbossServerDeployment {
	serverGroup, ok := jbossFindServerGroup(basePathFs, serverName)
	if !ok {
		return nil
	}
	file, err := basePathFs.Open(filepath.Join(jbossConfigDir, configFile))
	if err != nil {
		return nil
	}
	defer file.Close()
	var descriptor jbossDomainXML
	err = xml.NewDecoder(file).Decode(&descriptor)
	if err != nil {
		return nil
	}

	var currentGroup *jbossServerGroup
	// find the deployments enabled matching the current server group
	for _, group := range descriptor.ServerGroups {
		if group.Name == serverGroup {
			currentGroup = &group
			break
		}
	}
	if currentGroup == nil {
		return nil
	}
	// index deployments for faster lookup
	indexed := make(map[string]jbossServerDeployment, len(descriptor.Deployments))
	for _, deployment := range descriptor.Deployments {
		indexed[deployment.Name] = deployment
	}

	ret := make([]jbossServerDeployment, 0, len(indexed))
	for _, deployment := range currentGroup.Deployments {
		if !XMLStringToBool(deployment.Enabled) {
			continue
		}
		value, ok := indexed[deployment.Name]
		if !ok {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}

// jbossStandaloneFindDeployments finds active deployments for a server in standalone mode.
func jbossStandaloneFindDeployments(basePathFs afero.Fs, configFile string) []jbossServerDeployment {
	file, err := basePathFs.Open(filepath.Join(jbossConfigDir, configFile))
	if err != nil {
		return nil
	}
	defer file.Close()
	var descriptor jbossStandaloneXML
	err = xml.NewDecoder(file).Decode(&descriptor)
	if err != nil {
		return nil
	}
	var ret = make([]jbossServerDeployment, 0, len(descriptor.Deployments))
	for _, value := range descriptor.Deployments {
		if len(value.Enabled) == 0 || XMLStringToBool(value.Enabled) {
			ret = append(ret, value)
		}
	}
	return ret
}

// jbossExtractServerName extracts the server name from the command line.
func jbossExtractServerName(args []string) (string, bool) {
	value, ok := extractJavaPropertyFromArgs(args, jbossServerName)
	if !ok || len(value) <= 1 {
		return "", false
	}
	// the property is on the form -D[Server:servername]. Closing bracket is removed
	return value[:len(value)-1], true
}

// jbossFindServerGroup parses host.xml file and finds the server group where the provided serverName belongs to.
// domainFs should be the relative fs rooted to the domain home
func jbossFindServerGroup(domainFs afero.Fs, serverName string) (string, bool) {
	file, err := domainFs.Open(filepath.Join(jbossConfigDir, jbossHostXMLFile))
	if err != nil {
		return "", false
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	var decoded jbossHostXML
	err = decoder.Decode(&decoded)
	if err != nil || len(decoded.Servers) == 0 {
		return "", false
	}
	for _, server := range decoded.Servers {
		if server.Name == serverName {
			return server.Group, true
		}
	}
	return "", false
}
