// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package usm

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
)

// TestResolveAppServerFromCmdLine tests that vendor can be determined from the process cmdline
func TestResolveAppServerFromCmdLine(t *testing.T) {
	tests := []struct {
		name           string
		rawCmd         string
		expectedVendor serverVendor
		expectedHome   string
	}{
		{
			name: "wildfly 18 standalone",
			rawCmd: `/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java -D[Standalone] -server
-Xms64m -Xmx512m -XX:MetaspaceSize=96M -XX:MaxMetaspaceSize=256m -Djava.net.preferIPv4Stack=true
-Djboss.modules.system.pkgs=org.jboss.byteman -Djava.awt.headless=true
--add-exports=java.base/sun.nio.ch=ALL-UNNAMED --add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED
--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED -Dorg.jboss.boot.log.file=/home/app/Downloads/wildfly-18.0.0.Final/standalone/log/server.log
-Dlogging.configuration=file:/home/app/Downloads/wildfly-18.0.0.Final/standalone/configuration/logging.properties
-jar /home/app/Downloads/wildfly-18.0.0.Final/jboss-modules.jar -mp /home/app/Downloads/wildfly-18.0.0.Final/modules org.jboss.as.standalone
-Djboss.home.dir=/home/app/Downloads/wildfly-18.0.0.Final -Djboss.server.base.dir=/home/app/Downloads/wildfly-18.0.0.Final/standalone`,
			expectedVendor: jboss,
			expectedHome:   "/home/app/Downloads/wildfly-18.0.0.Final/standalone",
		},
		{
			name: "wildfly 18 domain",
			rawCmd: `/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java --add-exports=java.base/sun.nio.ch=ALL-UNNAMED
--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED --add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED -D[Server:server-one]
-D[pcid:780891833] -Xms64m -Xmx512m -server -XX:MetaspaceSize=96m -XX:MaxMetaspaceSize=256m -Djava.awt.headless=true -Djava.net.preferIPv4Stack=true
-Djboss.home.dir=/home/app/Downloads/wildfly-18.0.0.Final -Djboss.modules.system.pkgs=org.jboss.byteman
-Djboss.server.log.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/log
-Djboss.server.temp.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/tmp
-Djboss.server.data.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/data
-Dorg.jboss.boot.log.file=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/log/server.log
-Dlogging.configuration=file:/home/app/Downloads/wildfly-18.0.0.Final/domain/configuration/default-server-logging.properties
-jar /home/app/Downloads/wildfly-18.0.0.Final/jboss-modules.jar -mp /home/app/Downloads/wildfly-18.0.0.Final/modules org.jboss.as.server`,
			expectedVendor: jboss,
			expectedHome:   "/home/app/Downloads/wildfly-18.0.0.Final/domain",
		},
		{
			name: "tomcat 10.x",
			rawCmd: `java -Djava.util.logging.config.file=/app/Code/tomcat/apache-tomcat-10.0.27/conf/logging.properties
-Djava.util.logging.manager=org.apache.juli.ClassLoaderLogManager -Djdk.tls.ephemeralDHKeySize=2048
-Djava.protocol.handler.pkgs=org.apache.catalina.webresources -Dorg.apache.catalina.security.SecurityListener.UMASK=0027
-Dignore.endorsed.dirs= -classpath /app/Code/tomcat/apache-tomcat-10.0.27/bin/bootstrap.jar:/app/Code/tomcat/apache-tomcat-10.0.27/bin/tomcat-juli.jar
-Dcatalina.base=/app/Code/tomcat/apache-tomcat-10.0.27/myserver -Dcatalina.home=/app/Code/tomcat/apache-tomcat-10.0.27
-Djava.io.tmpdir=/app/Code/tomcat/apache-tomcat-10.0.27/temp org.apache.catalina.startup.Bootstrap start`,
			expectedVendor: tomcat,
			expectedHome:   "/app/Code/tomcat/apache-tomcat-10.0.27/myserver",
		},
		{
			name: "weblogic 12",
			rawCmd: `/u01/jdk/bin/java -Djava.security.egd=file:/dev/./urandom -cp /u01/oracle/wlserver/server/lib/weblogic-launcher.jar
-Dlaunch.use.env.classpath=true -Dweblogic.Name=AdminServer -Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy
-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader -javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar
-da -Dwls.home=/u01/oracle/wlserver/server -Dweblogic.home=/u01/oracle/wlserver/server weblogic.Server`,
			expectedVendor: weblogic,
		},
		{
			name: "websphere traditional 9.x",
			rawCmd: `/opt/IBM/WebSphere/AppServer/java/8.0/bin/java -Dosgi.install.area=/opt/IBM/WebSphere/AppServer
-Dwas.status.socket=43471 -Dosgi.configuration.area=/opt/IBM/WebSphere/AppServer/profiles/AppSrv01/servers/server1/configuration
-Djava.awt.headless=true -Dosgi.framework.extensions=com.ibm.cds,com.ibm.ws.eclipse.adaptors
-Xshareclasses:name=webspherev9_8.0_64_%g,nonFatal -Dcom.ibm.xtq.processor.overrideSecureProcessing=true -Xcheck:dump
-Djava.security.properties=/opt/IBM/WebSphere/AppServer/properties/java.security -Djava.security.policy=/opt/IBM/WebSphere/AppServer/properties/java.policy
-Dcom.ibm.CORBA.ORBPropertyFilePath=/opt/IBM/WebSphere/AppServer/properties -Xbootclasspath/p:/opt/IBM/WebSphere/AppServer/java/8.0/jre/lib/ibmorb.jar
-classpath /opt/IBM/WebSphere/AppServer/profiles/AppSrv01/properties:/opt/IBM/WebSphere/AppServer/properties:/opt/IBM/WebSphere/AppServer/lib/startup.jar:shortened.jar
-Dibm.websphere.internalClassAccessMode=allow -Xms50m -Xmx1962m -Xcompressedrefs -Xscmaxaot12M -Xscmx90M
-Dws.ext.dirs=/opt/IBM/WebSphere/AppServer/java/8.0/lib:/opt/IBM/WebSphere/AppServer/profiles/AppSrv01/classes:shortened
-Dderby.system.home=/opt/IBM/WebSphere/AppServer/derby -Dcom.ibm.itp.location=/opt/IBM/WebSphere/AppServer/bin
-Djava.util.logging.configureByServer=true -Duser.install.root=/opt/IBM/WebSphere/AppServer/profiles/AppSrv01
-Djava.ext.dirs=/opt/IBM/WebSphere/AppServer/tivoli/tam:/opt/IBM/WebSphere/AppServer/javaext:/opt/IBM/WebSphere/AppServer/java/8.0/jre/lib/ext
-Djavax.management.builder.initial=com.ibm.ws.management.PlatformMBeanServerBuilder -Dwas.install.root=/opt/IBM/WebSphere/AppServer
-Djava.util.logging.manager=com.ibm.ws.bootstrap.WsLogManager -Dserver.root=/opt/IBM/WebSphere/AppServer/profiles/AppSrv01
-Dcom.ibm.security.jgss.debug=off -Dcom.ibm.security.krb5.Krb5Debug=off -Djava.util.prefs.userRoot=/home/was/ -Xnoloa
-Djava.library.path=/opt/IBM/WebSphere/AppServer/lib/native/linux/x86_64/:/opt/IBM/WebSphere/AppServer/java/8.0/jre/lib/amd64/compressedrefs:shortened
com.ibm.wsspi.bootstrap.WSPreLauncher -nosplash -application com.ibm.ws.bootstrap.WSLauncher com.ibm.ws.runtime.WsServer
/opt/IBM/WebSphere/AppServer/profiles/AppSrv01/config DefaultCell01 DefaultNode01 server1`,
			expectedHome:   "/opt/IBM/WebSphere/AppServer/profiles/AppSrv01",
			expectedVendor: websphere,
		},
		{
			// weblogic cli have the same system properties than normal weblogic server run (sourced from setWlsEnv.sh)
			// however, the main entry point changes (weblogic.Deployer) hence should be recognized as unknown
			name: "weblogic deployer",
			rawCmd: `/u01/jdk/bin/java -Djava.security.egd=file:/dev/./urandom -cp /u01/oracle/wlserver/server/lib/weblogic-launcher.jar
-Dlaunch.use.env.classpath=true -Dweblogic.Name=AdminServer -Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy
-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader -javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar
-da -Dwls.home=/u01/oracle/wlserver/server -Dweblogic.home=/u01/oracle/wlserver/server weblogic.Deployer -upload -target myserver -deploy some.war`,
			expectedVendor: unknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := strings.Split(strings.ReplaceAll(tt.rawCmd, "\n", " "), " ")
			vendor, home := jeeExtractor{NewDetectionContext(cmd, envs.NewVariables(nil), fstest.MapFS{})}.resolveAppServer()
			require.Equal(t, tt.expectedVendor, vendor)
			// the base dir is making sense only when the vendor has been properly understood
			if tt.expectedVendor != unknown {
				require.Equal(t, tt.expectedHome, home)
			}
		})
	}
}

// TestExtractContextRootFromApplicationXml tests that context root can be extracted from an ear under /META-INF/application.xml
func TestExtractContextRootFromApplicationXml(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected []string
		err      bool
	}{
		{
			name: "application.xml with webapps",
			xml: `<application xmlns="http://xmlns.jcp.org/xml/ns/javaee" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.jcp.org/xml/ns/javaee http://xmlns.jcp.org/xml/ns/javaee/application_7.xsd" version="7">
	<application-name>myapp</application-name>
	<initialize-in-order>false</initialize-in-order>
  	<module><ejb>mymodule.jar</ejb></module>
  <module>
        <web>
            <web-uri>myweb1.war</web-uri>
            <context-root>MyWeb1</context-root>
        </web>
    </module>
	<module>
        <web>
            <web-uri>myweb2.war</web-uri>
            <context-root>MyWeb2</context-root>
        </web>
    </module>
</application>`,
			expected: []string{"MyWeb1", "MyWeb2"},
		},
		{
			name: "application.xml with doctype and no webapps",
			xml: `<!DOCTYPE application PUBLIC "-//Sun Microsystems, Inc.//DTD J2EE Application 1.2//EN
http://java.sun.com/j2ee/dtds/application_1_2.dtd">
<application><module><java>my_app.jar</java></module></application>`,
			expected: nil,
		},
		{
			name: "no application.xml (invalid ear)",
			err:  true,
		},
		{
			name: "invalid application.xml (invalid ear)",
			err:  true,
			xml:  "invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapFs := fstest.MapFS{}

			if len(tt.xml) > 0 {
				mapFs[applicationXMLPath] = &fstest.MapFile{Data: []byte(tt.xml)}
			}
			value, err := extractContextRootFromApplicationXML(mapFs)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, value)
			}
		})
	}
}

// TestWeblogicExtractServiceNamesForJEEServer tests all cases of detecting weblogic as vendor and extracting context root.
// It simulates having 1 ear deployed, 1 war with weblogic.xml and 1 war without weblogic.xml.
// Hence, it should extract ear context from application.xml, 1st war context from weblogic.xml and derive last war context from the filename.
func TestWeblogicExtractServiceNamesForJEEServer(t *testing.T) {
	wlsConfig := `
<domain>
    <app-deployment>
        <target>AdminServer</target>
        <source-path>apps/app1.ear</source-path>
        <staging-mode>stage</staging-mode>
    </app-deployment>
    <app-deployment>
        <target>AdminServer</target>
        <source-path>apps/app2.war</source-path>
        <staging-mode>stage</staging-mode>
    </app-deployment>
    <app-deployment>
        <target>AdminServer</target>
        <source-path>apps/app3.war</source-path>
        <staging-mode>stage</staging-mode>
    </app-deployment>
</domain>`
	appXML := `
<application>
  <application-name>myapp</application-name>
  <initialize-in-order>false</initialize-in-order>
  <module>
	<web>
      <web-uri>app1.war</web-uri>
      <context-root>app1_context</context-root>
    </web>
  </module>
</application>`
	weblogicXML := `
<weblogic-web-app>
   <context-root>app2_context</context-root>
</weblogic-web-app>
`
	buf := bytes.NewBuffer([]byte{})
	writer := zip.NewWriter(buf)
	require.NoError(t, writeFile(writer, weblogicXMLFile, weblogicXML))
	require.NoError(t, writer.Close())
	memfs := &fstest.MapFS{
		"wls/domain/config/config.xml":                   &fstest.MapFile{Data: []byte(wlsConfig)},
		"wls/domain/apps/app1.ear/" + applicationXMLPath: &fstest.MapFile{Data: []byte(appXML)},
		"wls/domain/apps/app2.war":                       &fstest.MapFile{Data: buf.Bytes()},
		"wls/domain/apps/app3.war":                       &fstest.MapFile{Mode: fs.ModeDir},
	}

	// simulate weblogic command line args
	cmd := []string{
		wlsServerNameSysProp + "AdminServer",
		wlsHomeSysProp + "/wls",
		wlsServerMainClass,
	}
	envsMap := map[string]string{
		"PWD": "wls/domain",
	}
	extractor := jeeExtractor{ctx: NewDetectionContext(cmd, envs.NewVariables(envsMap), memfs)}
	extractedContextRoots := extractor.extractServiceNamesForJEEServer()
	require.Equal(t, []string{
		"app1_context", // taken from ear application.xml
		"app2_context", // taken from war weblogic.xml
		"app3",         // derived from the war filename
	}, extractedContextRoots)
}

// TestNormalizeContextRoot runs tests cases for context root normalization.
func TestNormalizeContextRoot(t *testing.T) {
	tests := []struct {
		name     string
		arg      []string
		expected []string
	}{
		{
			name: "Should strip / from context roots",
			arg: []string{
				"/test1",
				"test2",
				"/test3/test4",
			},
			expected: []string{
				"test1",
				"test2",
				"test3/test4",
			},
		},
		{
			name: "should handle empty slices",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, normalizeContextRoot(tt.arg...))
		})
	}
}

func TestServerVendorStringer(t *testing.T) {
	require.Equal(t, "tomcat", tomcat.String())
	require.Equal(t, "weblogic", weblogic.String())
	require.Equal(t, "websphere", websphere.String())
	require.Equal(t, "jboss", jboss.String())
	require.Equal(t, "unknown", unknown.String())
	require.Equal(t, "unknown", serverVendor(123).String())
}
