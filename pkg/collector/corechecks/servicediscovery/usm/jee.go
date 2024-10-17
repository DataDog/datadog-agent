// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// appserver is an enumeration of application server types
type serverVendor uint8

// appserver bitwise enums. Each element should be a power of two. The first element, unknown is 0.
const (
	unknown serverVendor = 0
	jboss   serverVendor = 1 << (iota - 1)
	tomcat
	weblogic
	websphere
)

type (
	// deploymentType is an enum to describe the type of deployment (can be ear or war)
	deploymentType uint8

	// vendorExtractor defines interfaces to extract context roots from a jee server
	vendorExtractor interface {
		// findDeployedApps is used to find the application deployed on a domainHome
		findDeployedApps(domainHome string) ([]jeeDeployment, bool)
		// customExtractWarContextRoot is used to extract the context root from a vendor defined configuration inside the war.
		// if not found it returns en empty string and false
		customExtractWarContextRoot(fs fs.FS) (string, bool)
		// defaultContextRootFromFile returns the default naming that apply for a certain fileName.
		// it is usually the file without the extension, but it can differ for some vendors (i.e. tomcat)
		defaultContextRootFromFile(fileName string) (string, bool)
	}
)

// definitions of standard extractors
var extractors = map[serverVendor]func(DetectionContext) vendorExtractor{
	jboss:     newJbossExtractor,
	weblogic:  newWeblogicExtractor,
	websphere: newWebsphereExtractor,
	tomcat:    newTomcatExtractor,
}

const (
	war deploymentType = iota + 1
	ear
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
	jbossBaseDirSysProp  string = "-Djboss.server.base.dir="
	julConfigSysProp     string = "-Dlogging.configuration="
	applicationXMLPath   string = "META-INF/application.xml"
)

type (
	// applicationXML is used to unmarshal information from a standard EAR's application.xml
	// example doc: https://docs.oracle.com/cd/E13222_01/wls/docs61/programming/app_xml.html
	applicationXML struct {
		XMLName     xml.Name `xml:"application"`
		ContextRoot []string `xml:"module>web>context-root"`
	}

	// typedDeployment describes a deployment with a deploymentType if early detected
	jeeDeployment struct {
		name        string
		path        string
		dt          deploymentType
		contextRoot string
	}
)

// fileSystemCloser wraps a FileSystem with a Closer in case the filesystem has been created with a stream that
// should be closed after its usage.
type fileSystemCloser struct {
	fs     fs.FS
	closer io.Closer
}

type jeeExtractor struct {
	ctx DetectionContext
}

// extractContextRootFromApplicationXML parses a standard application.xml file extracting
// mount points for web application (aka context roots).
func extractContextRootFromApplicationXML(fs fs.FS) ([]string, error) {
	file, err := fs.Open(applicationXMLPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := SizeVerifiedReader(file)
	if err != nil {
		return nil, err
	}
	var a applicationXML
	err = xml.NewDecoder(reader).Decode(&a)
	if err != nil {
		return nil, err
	}
	return a.ContextRoot, nil
}

// resolveAppServerFromC parses the command line and tries to extract a couple of evidences for each known application server.
// This function only return a serverVendor if both hints are matching the same vendor.
// The first hint is about the server home that's typically different from vendor to vendor
// The second hint is about the entry point (i.e. the main class name) that's bootstrapping the server
// The reasons why we need both hints to match is that, in some cases the same jar may be used for admin operations (not to launch the server)
// or the same property may be used for admin operation and not to launch the server (like happening for weblogic).
// In case the vendor is matched, the server baseDir is also returned, otherwise the vendor unknown is returned
func (je jeeExtractor) resolveAppServer() (serverVendor, string) {
	serverHomeHint, entrypointHint := unknown, unknown
	var baseDir string
	// jboss in domain mode does not expose the domain base dir but that path can be derived from the logging configuration
	var julConfigFile string
	for _, a := range je.ctx.Args {
		if serverHomeHint == unknown {
			switch {
			case strings.HasPrefix(a, wlsHomeSysProp):
				// use the CWD for weblogic since the wlsHome is the home of the weblogic installation and not of the domain
				serverHomeHint = weblogic
			case strings.HasPrefix(a, tomcatSysProp):
				serverHomeHint = tomcat
				baseDir = strings.TrimPrefix(a, tomcatSysProp)
			case strings.HasPrefix(a, jbossBaseDirSysProp):
				serverHomeHint = jboss
				baseDir = strings.TrimPrefix(a, jbossBaseDirSysProp)
			case strings.HasPrefix(a, julConfigSysProp):
				// take the value for this property but continue parsing to give a chance to find the explicit basedir
				// that config value can be a uri so trim file: if present
				julConfigFile = strings.TrimPrefix(strings.TrimPrefix(a, julConfigSysProp), "file:")
			case strings.HasPrefix(a, websphereHomeSysProp):
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
	if serverHomeHint == unknown && entrypointHint == jboss && len(julConfigFile) > 0 {
		// if we cannot find the basedir (happens by default on jboss domain home), we derive it from the jul logging config location
		baseDir = path.Dir(path.Dir(julConfigFile))
		serverHomeHint = jboss
	}
	return serverHomeHint & entrypointHint, baseDir
}

// standardExtractContextFromWarName is the standard algorithm to deduce context root from war name.
// It returns the filename (or directory name if the deployment is exploded) without the extension
func standardExtractContextFromWarName(fileName string) (string, bool) {
	dir, file := path.Split(fileName)
	f := file
	if len(f) == 0 {
		f = dir
	}
	return strings.TrimSuffix(f, path.Ext(f)), true
}

// vfsAndTypeFromAppPath inspects the appPath and returns a valid fileSystemCloser in case the deployment is an ear or a war.
func vfsAndTypeFromAppPath(deployment *jeeDeployment, filesystem fs.SubFS) (*fileSystemCloser, deploymentType, error) {
	dt := deployment.dt
	if dt == 0 {
		ext := strings.ToLower(path.Clean(path.Ext(deployment.name)))
		switch ext {
		case ".ear":
			dt = ear
		case ".war":
			dt = war
		default:
			return nil, dt, fmt.Errorf("unhandled deployment type %s", ext)
		}

	}
	fi, err := fs.Stat(filesystem, deployment.path)
	if err != nil {
		return nil, dt, err
	}

	if fi.IsDir() {
		sub, err := filesystem.Sub(deployment.path)
		if err != nil {
			return nil, dt, err
		}
		return &fileSystemCloser{
			fs: sub,
		}, dt, nil
	}
	f, err := filesystem.Open(deployment.path)
	if err != nil {
		return nil, dt, err
	}

	// Re-stat after opening to avoid races with attributes changing before
	// previous stat and open.
	fi, err = f.Stat()
	if err != nil {
		return nil, dt, err
	}
	if !fi.Mode().IsRegular() {
		return nil, dt, err
	}

	r, err := zip.NewReader(f.(io.ReaderAt), fi.Size())
	if err != nil {
		_ = f.Close()
		return nil, dt, err
	}
	return &fileSystemCloser{
		fs:     r,
		closer: f,
	}, dt, nil
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
func (je jeeExtractor) doExtractContextRoots(extractor vendorExtractor, app *jeeDeployment) []string {
	log.Debugf("extracting context root (%q) for a jee application (%q)", app.path, app.name)
	if len(app.contextRoot) > 0 {
		return []string{app.contextRoot}
	}
	fsCloser, dt, err := vfsAndTypeFromAppPath(app, je.ctx.fs)
	if err != nil {
		log.Debugf("error locating the deployment: %v", err)
		if dt == ear {
			return nil
		}
		value, ok := extractor.defaultContextRootFromFile(app.name)
		if ok {
			return []string{value}
		}
		return nil
	}
	if fsCloser.closer != nil {
		defer fsCloser.closer.Close()
	}
	if dt == ear {
		value, err := extractContextRootFromApplicationXML(fsCloser.fs)
		if err != nil {
			log.Debugf("unable to extract context roots from application.xml: %v", err)
			return nil
		}
		return value
	}
	if value, ok := extractor.customExtractWarContextRoot(fsCloser.fs); ok {
		return []string{value}
	}
	value, ok := extractor.defaultContextRootFromFile(app.name)
	if ok {
		return []string{value}
	}
	return nil
}

// extractServiceNamesForJEEServer takes args, cws and the fs (for testability reasons) and, after having determined the vendor,
// If the vendor can be determined, it returns the context roots if found, otherwise the server name.
// If the vendor is unknown, it returns a nil slice
func (je jeeExtractor) extractServiceNamesForJEEServer() []string {
	vendor, domainHome := je.resolveAppServer()
	if vendor == unknown {
		return nil
	}
	log.Debugf("running java enterprise service extraction - vendor %q", vendor)
	// check if able to find which applications are deployed
	extractorCreator, ok := extractors[vendor]
	if !ok {
		return nil
	}
	extractor := extractorCreator(je.ctx)
	cwd, ok := workingDirFromEnvs(je.ctx.Envs)
	if ok {
		domainHome = abs(domainHome, cwd)
	}

	apps, ok := extractor.findDeployedApps(domainHome)
	if !ok {
		return nil
	}
	var contextRoots []string
	for _, app := range apps {
		contextRoots = append(contextRoots, normalizeContextRoot(je.doExtractContextRoots(extractor, &app)...)...)
	}
	if len(contextRoots) == 0 {
		return nil
	}
	return contextRoots
}

func (s serverVendor) String() string {
	switch s {
	case jboss:
		return "jboss"
	case tomcat:
		return "tomcat"
	case weblogic:
		return "weblogic"
	case websphere:
		return "websphere"
	default:
		return "unknown"
	}
}

// extractJavaPropertyFromArgs loops through the command argument to see if a system property declaration matches the provided name.
// name should be in the form of `-D<property_name>=` (`-D` prolog and `=` epilogue) to avoid concatenating strings on each function call.
// The function returns the property value if found and a bool (true if found, false otherwise)
func extractJavaPropertyFromArgs(args []string, name string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, name) {
			return strings.TrimPrefix(a, name), true
		}
	}
	return "", false
}
