// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package javaparser contains functions to autodetect service name for java applications
package javaparser

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/afero/zipfs"
)

// appserver is an enumeration of application server types
type serverVendor uint8

// appserver bitwise enums. Each element should be a power of two. The first element, unknown is 0.
const (
	unknown serverVendor = 0
	jboss                = 1 << (iota - 1)
	tomcat
	weblogic
	websphere
)

const (
	// app servers hints
	wlsServerMainClass   string = "weblogic.Server"
	wlsHomeSysProp       string = "-Dwls.home="
	websphereHomeSysProp string = "-Dserver.root="
	websphereMainClass   string = "com.ibm.ws.runtime.WsServer"
	tomcatMainClass      string = "org.apache.catalina.startup.Bootstrap"
	tomcatSysProp        string = "-Dcatalina.base="
	jbossStandaloneMain  string = "org.jboss.as.standalone"
	jbossDomainMain      string = "org.jboss.as.server"
	jbossSysProp         string = "-Djboss.home.dir="
	applicationXMLPath   string = "/META-INF/application.xml"
)

type (
	// applicationXML is used to unmarshal information from a standard EAR's application.xml
	// example doc: https://docs.oracle.com/cd/E13222_01/wls/docs61/programming/app_xml.html
	applicationXML struct {
		XMLName     xml.Name `xml:"application"`
		ContextRoot []string `xml:"module>web>context-root"`
	}
	// deployedAppFindFn is used to find the application deployed on a domainHome
	// args should be supplied since some vendors may require additional information from them (i.e. server name)
	deployedAppFindFn func(domainHome string, args []string, fs afero.Fs) ([]string, bool)
	// warContextRootFindFn is used to extract the context root from a vendor defined configuration inside the war.
	// if not found it returns en empty string and false
	warContextRootFindFn func(fs afero.Fs) (string, bool)
	// defaultWarContextRootFn returns the default naming that apply for a certain fileName.
	// it is usually the file without the extension, but it can differ for some vendors (i.e. tomcat)
	defaultWarContextRootFn func(fileName string) string
)

// definitions of standard extractors
var (
	deploymentFinders = map[serverVendor]deployedAppFindFn{
		weblogic: weblogicFindDeployedApps,
	}
	contextRootFinders = map[serverVendor]warContextRootFindFn{
		weblogic: weblogicExtractWarContextRoot,
	}
	defaultContextNameExtractors = map[serverVendor]defaultWarContextRootFn{
		weblogic: standardExtractContextFromWarName,
	}
)

// extractContextRootFromApplicationXML parses a standard application.xml file extracting
// mount points for web application (aka context roots).
func extractContextRootFromApplicationXML(fs afero.Fs) ([]string, error) {
	reader, err := fs.Open(applicationXMLPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var a applicationXML
	err = xml.NewDecoder(reader).Decode(&a)
	if err != nil {
		return nil, err
	}
	return a.ContextRoot, nil
}

// resolveAppServerFromCmdLine parses the command line and tries to extract a couple of evidences for each known application server.
// This function only return a serverVendor if both hints are matching the same vendor.
// The first hint is about the server home that's typically different from vendor to vendor
// The second hint is about the entry point (i.e. the main class name) that's bootstrapping the server
// The reasons why we need both hints to match is that, in some cases the same jar may be used for admin operations (not to launch the server)
// or the same property may be used for admin operation and not to launch the server (like happening for weblogic).
// In case the vendor is matched, the server baseDir is also returned, otherwise the vendor unknown is returned
func resolveAppServerFromCmdLine(args []string) (serverVendor, string) {
	serverHomeHint, entrypointHint := unknown, unknown
	var baseDir string
	for _, a := range args {
		if serverHomeHint == unknown {
			if strings.HasPrefix(a, wlsHomeSysProp) {
				// use the CWD for weblogic since the wlsHome is the home of the weblogic installation and not of the domain
				serverHomeHint = weblogic
			} else if strings.HasPrefix(a, tomcatSysProp) {
				serverHomeHint = tomcat
				baseDir = strings.TrimPrefix(a, tomcatSysProp)
			} else if strings.HasPrefix(a, jbossSysProp) {
				serverHomeHint = jboss
				baseDir = strings.TrimPrefix(a, jbossSysProp)
			} else if strings.HasPrefix(a, websphereHomeSysProp) {
				serverHomeHint = websphere
				baseDir = strings.TrimPrefix(a, websphereHomeSysProp)
			}
		}
		if entrypointHint == unknown {
			// only return a match if it's exact meaning that the hint and the evidence are matching the same server type.
			switch a {
			case wlsServerMainClass:
				entrypointHint = weblogic
			case tomcatMainClass:
				entrypointHint = tomcat
			case websphereMainClass:
				entrypointHint = websphere
			case jbossDomainMain, jbossStandaloneMain:
				entrypointHint = jboss
			}
		}
		if serverHomeHint&entrypointHint != unknown {
			break
		}
	}
	return serverHomeHint & entrypointHint, baseDir
}

// standardExtractContextFromWarName is the standard algorithm to deduce context root from war name.
// It returns the filename (or directory name if the deployment is exploded) without the extension
func standardExtractContextFromWarName(fileName string) string {
	dir, file := filepath.Split(fileName)
	f := file
	if len(f) == 0 {
		f = dir
	}
	return strings.TrimSuffix(f, filepath.Ext(f))
}

// vfsAndTypeFromAppPath inspects the appPath and returns a valid fileSystemCloser in case the deployment is an ear or a war.
func vfsAndTypeFromAppPath(appPath string, fs afero.Fs) (*fileSystemCloser, bool, error) {
	ext := strings.ToLower(filepath.Clean(filepath.Ext(appPath)))
	isEar := false
	if ext == ".ear" {
		isEar = true
	} else if ext != ".war" {
		// only ear and war are supported
		return nil, false, fmt.Errorf("unhandled deployment type %s", ext)
	}
	fi, err := fs.Stat(appPath)
	if err != nil {
		return nil, isEar, err
	}

	if fi.IsDir() {
		return &fileSystemCloser{
			fs: afero.NewBasePathFs(fs, appPath),
		}, isEar, nil
	}
	f, err := fs.Open(appPath)
	if err != nil {
		return nil, false, err
	}
	r, err := zip.NewReader(f, fi.Size())
	if err != nil {
		_ = f.Close()
		return nil, isEar, err
	}
	return &fileSystemCloser{
		fs: zipfs.New(r),
		cf: f.Close,
	}, isEar, nil
}

// serviceName translate service vendor enumeration to the service name tag. Returns empty if not supported
func defaultIfNoContextRoots(s serverVendor) []string {
	switch s {
	case jboss:
		return []string{"jboss"}
	case tomcat:
		return []string{"tomcat"}
	case weblogic:
		return []string{"weblogic"}
	case websphere:
		return []string{"websphere"}
	}
	return nil
}

// normalizeContextRoot applies the same normalization the java tracer does by removing the first / on the context-root if present.
func normalizeContextRoot(contextRoots ...string) []string {
	if len(contextRoots) == 0 {
		return contextRoots
	}
	normalized := make([]string, len(contextRoots))
	for i, s := range contextRoots {
		normalized[i] = strings.TrimPrefix(s, "/")
	}
	return normalized
}

// doExtractContextRoots tries to extract context roots for an app, given the vendor and the fs.
func doExtractContextRoots(vendor serverVendor, app string, fs afero.Fs) []string {
	fsCloser, ear, err := vfsAndTypeFromAppPath(app, fs)
	if err != nil {
		return nil
	}
	defer fsCloser.Close()
	if ear {
		value, err := extractContextRootFromApplicationXML(fsCloser.fs)
		if err != nil {
			return nil
		}
		return value
	}
	vendorWarFinder, ok := contextRootFinders[vendor]
	if ok {
		value, ok := vendorWarFinder(fsCloser.fs)
		if ok {
			return []string{value}
		}
	}
	defaultFinder, ok := defaultContextNameExtractors[vendor]
	if ok {
		return []string{defaultFinder(app)}
	}
	return nil
}

// ExtractServiceNamesForJEEServer takes args, cws and the fs (for testability reasons) and, after having determined the vendor,
// If the vendor can be determined, it returns the context roots if found, otherwise the server name.
// If the vendor is unknown, it returns a nil slice
func ExtractServiceNamesForJEEServer(args []string, cwd string, fs afero.Fs) []string {
	vendor, domainHome := resolveAppServerFromCmdLine(args)
	if vendor == unknown {
		return nil
	}
	// check if able to find which applications are deployed
	deploymentFinder, ok := deploymentFinders[vendor]
	if !ok {
		return defaultIfNoContextRoots(vendor)
	}
	if len(domainHome) == 0 {
		// for some servers this info is not available. Default to cwd
		domainHome = cwd
	}
	apps, ok := deploymentFinder(domainHome, args, fs)
	if !ok {
		return defaultIfNoContextRoots(vendor)
	}
	var contextRoots []string
	for _, app := range apps {
		contextRoots = append(contextRoots, normalizeContextRoot(doExtractContextRoots(vendor, app, fs)...)...)
	}
	if len(contextRoots) == 0 {
		return defaultIfNoContextRoots(vendor)
	}
	return contextRoots
}
