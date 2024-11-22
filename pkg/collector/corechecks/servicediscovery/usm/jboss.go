// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	jbossWebXMLFileMetaInf        = "META-INF/jboss-web.xml"
	jbossWebXMLFileWebInf         = "WEB-INF/jboss-web.xml"
)

type (
	jbossExtractor struct {
		cxt DetectionContext
	}

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

func newJbossExtractor(ctx DetectionContext) vendorExtractor {
	return &jbossExtractor{cxt: ctx}
}

// findDeployedApps is the entry point function to find deployed application on a jboss instance
// It detects if the instance is standalone or part of a cluster (domain). It returns a slice of jeeDeployment and a bool.
// That will be false in case no deployments have been found
func (j jbossExtractor) findDeployedApps(domainHome string) ([]jeeDeployment, bool) {
	baseDir, ok := extractJavaPropertyFromArgs(j.cxt.Args, jbossHomeDirSysProp)
	if !ok {
		log.Debug("jboss: unable to extract the home directory")
		return nil, false
	}
	// Add cwd if jboss.home.dir is relative. It's unclear if this is likely in
	// real life, but the tests do do it. JBoss/WildFly docs imply that this is
	// normally an absolute path (since it's set to JBOSS_HOME by default and a
	// lot of other paths are resolved relative to this one).
	if cwd, ok := workingDirFromEnvs(j.cxt.Envs); ok {
		baseDir = abs(baseDir, cwd)
	}
	serverName, domainMode := jbossExtractServerName(j.cxt.Args)
	if domainMode && len(serverName) == 0 {
		log.Debug("jboss: domain mode with missing server name")
		return nil, false
	}
	configFile := jbossExtractConfigFileName(j.cxt.Args, domainMode)
	var deployments []jbossServerDeployment
	var err error
	if domainMode {
		var sub fs.FS
		baseDir = path.Join(baseDir, jbossDomainBase)
		sub, err = j.cxt.fs.Sub(baseDir)
		if err != nil {
			log.Debugf("jboss: cannot open jboss home %q. Err: %v", baseDir, err)
			return nil, false
		}
		deployments, err = jbossDomainFindDeployments(sub, configFile, serverName)
	} else {
		var sub fs.FS
		baseDir = path.Join(baseDir, jbossStandaloneBase)
		sub, err = j.cxt.fs.Sub(baseDir)
		if err != nil {
			log.Debugf("jboss: cannot open jboss home %q. Err: %v", baseDir, err)
			return nil, false
		}
		deployments, err = jbossStandaloneFindDeployments(sub, configFile)
	}
	if err != nil || len(deployments) == 0 {
		log.Debugf("jboss: cannot find deployments. Err: %v", err)
		return nil, false
	}

	ret := make([]jeeDeployment, 0, len(deployments))
	for _, d := range deployments {
		// applications are in /data/xx/yyyyy (xx are the first two chars of the sha1)
		ret = append(ret, jeeDeployment{name: d.RuntimeName,
			path: path.Join(domainHome, jbossDataDir, jbossContentDir, d.Content.Hash[:2], d.Content.Hash[2:], jbossContentDir),
		})
	}
	return ret, true
}

// customExtractWarContextRoot inspects a war deployment in order to find the jboss-web.xml file under META-INF or WEB-INF.
// If a context root is found it's returned otherwise the function will return "", false
func (j jbossExtractor) customExtractWarContextRoot(warFS fs.FS) (string, bool) {
	file, err := warFS.Open(jbossWebXMLFileWebInf)
	if err != nil {
		// that file can be in WEB-INF or META-INF.
		file, err = warFS.Open(jbossWebXMLFileMetaInf)
		if err != nil {
			return "", false
		}

	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		log.Debugf("jboss: ignoring %q: %v", jbossWebXMLFileWebInf, err)
		return "", false
	}
	var jwx jbossWebXML
	if xml.NewDecoder(reader).Decode(&jwx) != nil || len(jwx.ContextRoot) == 0 {
		return "", false
	}
	return jwx.ContextRoot, true
}

func (j jbossExtractor) defaultContextRootFromFile(fileName string) (string, bool) {
	return standardExtractContextFromWarName(fileName)
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
		if _, val, ok := strings.Cut(a, "="); ok {
			// return the right part if separated by an equal
			return val
		}

		// return the next arg if separated by a space
		if len(args) > i {
			return args[i+1]
		}
	}
	return defaultConfig
}

// jbossDomainFindDeployments finds active deployments for a server in domain mode.
func jbossDomainFindDeployments(basePathFs fs.FS, configFile string, serverName string) ([]jbossServerDeployment, error) {
	serverGroup, ok, err := jbossFindServerGroup(basePathFs, serverName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("jboss: unable to find server group")
	}
	file, err := basePathFs.Open(path.Join(jbossConfigDir, configFile))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		log.Debugf("jboss: ignoring %q: %v", jbossWebXMLFileWebInf, err)
		return nil, err
	}
	var descriptor jbossDomainXML
	err = xml.NewDecoder(reader).Decode(&descriptor)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("jboss: unable to locate server group %s into domain.xml", serverGroup)
	}
	// index deployments for faster lookup
	indexed := make(map[string]jbossServerDeployment, len(descriptor.Deployments))
	for _, deployment := range descriptor.Deployments {
		indexed[deployment.Name] = deployment
	}

	ret := make([]jbossServerDeployment, 0, len(indexed))
	for _, deployment := range currentGroup.Deployments {
		if !xmlStringToBool(deployment.Enabled) {
			continue
		}
		value, ok := indexed[deployment.Name]
		if !ok {
			continue
		}
		ret = append(ret, value)
	}
	return ret, nil
}

// jbossStandaloneFindDeployments finds active deployments for a server in standalone mode.
func jbossStandaloneFindDeployments(basePathFs fs.FS, configFile string) ([]jbossServerDeployment, error) {
	file, err := basePathFs.Open(path.Join(jbossConfigDir, configFile))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		return nil, err
	}
	var descriptor jbossStandaloneXML
	err = xml.NewDecoder(reader).Decode(&descriptor)
	if err != nil {
		return nil, err
	}
	var ret = make([]jbossServerDeployment, 0, len(descriptor.Deployments))
	for _, value := range descriptor.Deployments {
		if len(value.Enabled) == 0 || xmlStringToBool(value.Enabled) {
			ret = append(ret, value)
		}
	}
	return ret, nil
}

// jbossExtractServerName extracts the server name from the command line.
func jbossExtractServerName(args []string) (string, bool) {
	value, ok := extractJavaPropertyFromArgs(args, jbossServerName)
	if !ok {
		return "", false
	}
	// the property is on the form -D[Server:servername]. Closing bracket is removed
	return value[:len(value)-1], true
}

// jbossFindServerGroup parses host.xml file and finds the server group where the provided serverName belongs to.
// domainFs should be the relative fs rooted to the domain home
func jbossFindServerGroup(domainFs fs.FS, serverName string) (string, bool, error) {
	file, err := domainFs.Open(path.Join(jbossConfigDir, jbossHostXMLFile))
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		return "", false, err
	}
	decoder := xml.NewDecoder(reader)
	var decoded jbossHostXML
	err = decoder.Decode(&decoded)
	if err != nil || len(decoded.Servers) == 0 {
		return "", false, err
	}
	for _, server := range decoded.Servers {
		if server.Name == serverName {
			return server.Group, true, nil
		}
	}
	return "", false, nil
}

// XMLStringToBool parses string element value and return false if explicitly set to `false` or `0`
func xmlStringToBool(s string) bool {
	switch strings.ToLower(s) {
	case "0", "false":
		return false
	}
	return true
}
