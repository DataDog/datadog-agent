package javaparser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			expectedHome:   "/home/app/Downloads/wildfly-18.0.0.Final",
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
			expectedHome:   "/home/app/Downloads/wildfly-18.0.0.Final",
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
			expectedHome:   "/u01/oracle/wlserver/server",
		},
		{
			name: "websphere",
			rawCmd: `/opt/java/openjdk/bin/java -javaagent:/opt/ol/wlp/bin/tools/ws-javaagent.jar -Djava.awt.headless=true
-Djdk.attach.allowAttachSelf=true --add-exportsjava.base/sun.security.action=ALL-UNNAMED --add-exportsjava.naming/com.sun.jndi.ldap=ALL-UNNAMED
--add-exportsjava.naming/com.sun.jndi.url.ldap=ALL-UNNAMED --add-exportsjdk.naming.dns/com.sun.jndi.dns=ALL-UNNAMED
--add-exportsjava.security.jgss/sun.security.krb5.internal=ALL-UNNAMED --add-exportsjdk.attach/sun.tools.attach=ALL-UNNAMED
--add-opensjava.base/java.util=ALL-UNNAMED --add-opensjava.base/java.lang=ALL-UNNAMED --add-opensjava.base/java.util.concurrent=ALL-UNNAMED
--add-opensjava.base/java.io=ALL-UNNAMED --add-opensjava.naming/javax.naming.spi=ALL-UNNAMED --add-opensjdk.naming.rmi/com.sun.jndi.url.rmi=ALL-UNNAMED
--add-opensjava.naming/javax.naming=ALL-UNNAMED --add-opensjava.rmi/java.rmi=ALL-UNNAMED --add-opensjava.sql/java.sql=ALL-UNNAMED
--add-opensjava.management/javax.management=ALL-UNNAMED --add-opensjava.base/java.lang.reflect=ALL-UNNAMED --add-opensjava.desktop/java.awt.image=ALL-UNNAMED
--add-opensjava.base/java.security=ALL-UNNAMED --add-opensjava.base/java.net=ALL-UNNAMED --add-opensjava.base/java.text=ALL-UNNAMED
--add-opensjava.base/sun.net.www.protocol.https=ALL-UNNAMED --add-exportsjdk.management.agent/jdk.internal.agent=ALL-UNNAMED
--add-exportsjava.base/jdk.internal.vm=ALL-UNNAMED -jar /opt/ol/wlp/bin/tools/ws-server.jar defaultServer`,
			expectedHome:   "",
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
			vendor, home := resolveAppServerFromCmdLine(strings.Split(strings.ReplaceAll(tt.rawCmd, "\n", " "), " "))
			require.Equal(t, tt.expectedVendor, vendor)
			// the base dir is making sense only when the vendor has been properly understood
			if tt.expectedVendor != unknown {
				require.Equal(t, tt.expectedHome, home)
			}
		})
	}
}
