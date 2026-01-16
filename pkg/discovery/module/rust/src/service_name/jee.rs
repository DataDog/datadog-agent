// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

mod jboss;
mod tomcat;
mod weblogic;
mod websphere;

use crate::fs::{SubDirFs, UnverifiedZipArchive};
use crate::procfs::Cmdline;
use crate::service_name::{DetectionContext, ServiceNameSource};
use normalize_path::NormalizePath;
use serde::Deserialize;
use std::io::{self, Read};
use std::path::{Path, PathBuf};
use thiserror::Error;

/// Error type for JEE service name extraction
#[derive(Debug, Error)]
pub enum Error {
    /// I/O error (file not found, permission denied, etc.)
    #[error("I/O error: {0}")]
    Io(#[from] io::Error),

    /// XML parsing error
    #[error("XML parse error: {0}")]
    XmlParse(String),

    /// Missing required configuration (property, file, etc.)
    #[error("Missing configuration: {0}")]
    MissingConfig(String),
}
// Constants for app server hints
const WLS_SERVER_MAIN_CLASS: &str = "weblogic.Server";
const WLS_HOME_SYS_PROP: &str = "-Dwls.home=";
const WEBSPHERE_HOME_SYS_PROP: &str = "-Dserver.root=";
const WEBSPHERE_MAIN_CLASS: &str = "com.ibm.ws.runtime.WsServer";
const TOMCAT_MAIN_CLASS: &str = "org.apache.catalina.startup.Bootstrap";
const TOMCAT_SYS_PROP: &str = "-Dcatalina.base=";
const JBOSS_STANDALONE_MAIN: &str = "org.jboss.as.standalone";
const JBOSS_DOMAIN_MAIN: &str = "org.jboss.as.server";
const JBOSS_BASE_DIR_SYS_PROP: &str = "-Djboss.server.base.dir=";
const JUL_CONFIG_SYS_PROP: &str = "-Dlogging.configuration=";
const APPLICATION_XML_PATH: &str = "META-INF/application.xml";

/// Server vendor enumeration
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ServerVendor {
    JBoss,
    Tomcat,
    WebLogic,
    WebSphere,
}

impl ServerVendor {
    fn as_source(&self) -> ServiceNameSource {
        match self {
            ServerVendor::JBoss => ServiceNameSource::Jboss,
            ServerVendor::Tomcat => ServiceNameSource::Tomcat,
            ServerVendor::WebLogic => ServiceNameSource::Weblogic,
            ServerVendor::WebSphere => ServiceNameSource::Websphere,
        }
    }
}

/// Deployment type (EAR or WAR)
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum DeploymentType {
    War,
    Ear,
}

/// Filesystem abstraction for accessing deployment files
/// Abstracts over both directory-based (exploded) and ZIP-based (packaged) deployments
enum DeploymentFs {
    Directory(SubDirFs),
    ZipArchive(UnverifiedZipArchive<cap_std::fs::File>),
}

impl DeploymentFs {
    /// Reads the entire contents of a file as a byte vector
    fn read_file_to_vec<P: AsRef<Path>>(&mut self, path: P) -> io::Result<Vec<u8>> {
        match self {
            DeploymentFs::Directory(fs) => {
                let file = fs.open(path)?;
                let mut reader = file.verify(None)?;
                let mut buf = Vec::new();
                reader.read_to_end(&mut buf)?;
                Ok(buf)
            }
            DeploymentFs::ZipArchive(zip) => {
                let path_str = path.as_ref().to_string_lossy();
                let entry = zip.by_name(&path_str)?;
                let mut reader = entry.verify(None)?;
                let mut buf = Vec::new();
                reader.read_to_end(&mut buf)?;
                Ok(buf)
            }
        }
    }
}

/// Represents a JEE deployment
#[derive(Default, Debug, Clone, PartialEq)]
struct Deployment {
    name: String,
    path: PathBuf,
    kind: Option<DeploymentType>,
    context_root: Option<String>,
}

/// Dispatch to vendor-specific find_deployed_apps implementation
fn find_deployed_apps(
    vendor: ServerVendor,
    cmdline: &Cmdline,
    ctx: &DetectionContext,
    domain_home: &Path,
) -> Result<Vec<Deployment>, Error> {
    match vendor {
        ServerVendor::JBoss => jboss::find_deployed_apps(cmdline, ctx, domain_home),
        ServerVendor::Tomcat => tomcat::find_deployed_apps(ctx, domain_home),
        ServerVendor::WebLogic => weblogic::find_deployed_apps(cmdline, ctx, domain_home),
        ServerVendor::WebSphere => websphere::find_deployed_apps(cmdline, ctx, domain_home),
    }
}

/// Dispatch to vendor-specific custom_extract_war_context_root implementation
/// Only JBoss and WebLogic have custom implementations
fn custom_extract_war_context_root(
    vendor: ServerVendor,
    deployment_fs: &mut DeploymentFs,
) -> Option<String> {
    match vendor {
        ServerVendor::JBoss => jboss::custom_extract_war_context_root(deployment_fs),
        ServerVendor::WebLogic => weblogic::custom_extract_war_context_root(deployment_fs),
        // Tomcat and WebSphere don't support vendor-specific context root extraction
        _ => None,
    }
}

/// Dispatch to vendor-specific default_context_root_from_file implementation
/// Only Tomcat has custom logic; others use standard extraction
fn default_context_root_from_file(vendor: ServerVendor, file_name: &str) -> Option<String> {
    match vendor {
        ServerVendor::Tomcat => tomcat::default_context_root_from_file(file_name),
        // All others use standard WAR name extraction
        _ => standard_extract_context_from_war_name(file_name),
    }
}

/// Extracts Java property value from command line arguments
/// The property name should include the `-D` prefix and `=` suffix (e.g., `-Dfoo.bar=`)
fn extract_java_property_from_args(cmdline: &Cmdline, property: &str) -> Option<String> {
    cmdline
        .args()
        .find_map(|arg| arg.strip_prefix(property))
        .map(|s| s.to_string())
}

/// Make path absolute relative to base
pub(super) fn abs(path: &str, base: &Path) -> PathBuf {
    let p = Path::new(path);
    if p.is_absolute() {
        p.to_path_buf()
    } else {
        // Go's `Path::Join()` `Clean()`s the path afterward, so we need to
        // normalize it here too for compatibility.
        base.join(p).normalize()
    }
}

/// Creates a DeploymentFs from a deployment path
/// Returns either a directory-based or ZIP-based filesystem depending on whether
/// the path points to a directory or a file
fn fs_from_deployment_path(fs: &SubDirFs, deployment_path: &Path) -> io::Result<DeploymentFs> {
    let metadata = fs.metadata(deployment_path)?;

    if metadata.is_dir() {
        // It's a directory - create a SubDirFs for it
        let sub_fs = fs.sub(deployment_path)?;
        Ok(DeploymentFs::Directory(sub_fs))
    } else if metadata.is_file() {
        // It's a file - open it as a ZIP archive
        let file = fs.open(deployment_path)?;
        let zip = file.verify_zip().map_err(|e| {
            io::Error::new(
                io::ErrorKind::InvalidData,
                format!("Failed to open ZIP archive: {}", e),
            )
        })?;
        Ok(DeploymentFs::ZipArchive(zip))
    } else {
        Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "Deployment path is neither a file nor a directory",
        ))
    }
}

/// Application.xml descriptor for EAR files
/// Example: https://docs.oracle.com/cd/E13222_01/wls/docs61/programming/app_xml.html
#[derive(Debug, Deserialize)]
struct ApplicationXml {
    #[serde(rename = "module", default)]
    modules: Vec<ApplicationModule>,
}

#[derive(Debug, Deserialize)]
struct ApplicationModule {
    #[serde(rename = "web", default)]
    web: Option<ApplicationWebModule>,
}

#[derive(Debug, Deserialize)]
struct ApplicationWebModule {
    #[serde(rename = "context-root")]
    context_root: String,
}

/// Extracts context roots from a standard EAR's application.xml
fn extract_context_root_from_application_xml(
    deployment_fs: &mut DeploymentFs,
) -> Result<Vec<String>, Error> {
    let xml_path = Path::new(APPLICATION_XML_PATH);
    let buf = deployment_fs.read_file_to_vec(xml_path)?;

    let app_xml: ApplicationXml = quick_xml::de::from_reader(buf.as_slice())
        .map_err(|e| Error::XmlParse(format!("Failed to parse application.xml: {}", e)))?;

    let context_roots: Vec<String> = app_xml
        .modules
        .into_iter()
        .filter_map(|m| m.web.map(|w| w.context_root))
        .collect();

    Ok(context_roots)
}

/// Standard algorithm to extract context root from WAR filename
fn standard_extract_context_from_war_name(file_name: &str) -> Option<String> {
    let path = Path::new(file_name);
    let name = path.file_name()?.to_str()?;
    let without_ext = name.strip_suffix(".war").or(Some(name))?;
    Some(without_ext.to_string())
}

/// normalize_context_root applies the same normalization the java tracer does
/// by removing the first / on the context-root if present.
fn normalize_context_root(context_roots: Vec<String>) -> Vec<String> {
    context_roots
        .into_iter()
        .map(|cr| cr.strip_prefix('/').unwrap_or(&cr).to_string())
        .collect()
}

/// Resolves the app server vendor and base directory from command line
/// resolve_app_server parses the command line and tries to extract a couple of
/// evidences for each known application server.
///
/// This function only return a serverVendor if both hints are matching the same
/// vendor.  The first hint is about the server home that's typically different
/// from vendor to vendor
///
/// The second hint is about the entry point (i.e. the main class name) that's
/// bootstrapping the server
///
/// The reasons why we need both hints to match is that, in some cases the same
/// jar may be used for admin operations (not to launch the server) or the same
/// property may be used for admin operation and not to launch the server (like
/// happening for weblogic).
///
/// In case the vendor is matched, the server baseDir is also returned,
/// otherwise the vendor unknown is returned
fn resolve_app_server(cmdline: &Cmdline) -> (Option<ServerVendor>, String) {
    let mut server_home_hint: Option<ServerVendor> = None;
    let mut entrypoint_hint: Option<ServerVendor> = None;
    let mut base_dir = String::new();
    let mut jul_config_file: Option<String> = None;

    for arg in cmdline.args() {
        // Check server home hints
        if server_home_hint.is_none() {
            if arg.starts_with(WLS_HOME_SYS_PROP) {
                server_home_hint = Some(ServerVendor::WebLogic);
                // For WebLogic, use CWD instead of wls.home
            } else if let Some(dir) = arg.strip_prefix(TOMCAT_SYS_PROP) {
                server_home_hint = Some(ServerVendor::Tomcat);
                base_dir = dir.to_string();
            } else if let Some(dir) = arg.strip_prefix(JBOSS_BASE_DIR_SYS_PROP) {
                server_home_hint = Some(ServerVendor::JBoss);
                base_dir = dir.to_string();
            } else if let Some(config) = arg.strip_prefix(JUL_CONFIG_SYS_PROP) {
                jul_config_file = Some(config.strip_prefix("file:").unwrap_or(config).to_string());
            } else if let Some(dir) = arg.strip_prefix(WEBSPHERE_HOME_SYS_PROP) {
                server_home_hint = Some(ServerVendor::WebSphere);
                base_dir = dir.to_string();
            }
        }

        // Check entrypoint hints
        if entrypoint_hint.is_none() {
            match arg {
                WLS_SERVER_MAIN_CLASS => entrypoint_hint = Some(ServerVendor::WebLogic),
                TOMCAT_MAIN_CLASS => entrypoint_hint = Some(ServerVendor::Tomcat),
                WEBSPHERE_MAIN_CLASS => entrypoint_hint = Some(ServerVendor::WebSphere),
                JBOSS_DOMAIN_MAIN | JBOSS_STANDALONE_MAIN => {
                    entrypoint_hint = Some(ServerVendor::JBoss)
                }
                _ => {}
            }
        }

        // Both hints match - we found the vendor
        if server_home_hint.is_some() && server_home_hint == entrypoint_hint {
            break;
        }
    }

    // Special case for JBoss domain mode: derive basedir from JUL config if not explicitly set
    if server_home_hint.is_none()
        && entrypoint_hint == Some(ServerVendor::JBoss)
        && let Some(jul_config) = jul_config_file
    {
        let config_path = Path::new(&jul_config);
        if let Some(parent) = config_path.parent().and_then(|p| p.parent()) {
            base_dir = parent.to_string_lossy().to_string();
            server_home_hint = Some(ServerVendor::JBoss);
        }
    }

    if server_home_hint.is_none() || server_home_hint != entrypoint_hint {
        return (None, base_dir);
    }

    (server_home_hint, base_dir)
}

/// Extracts service names for JEE servers
pub fn extract_names(
    cmdline: &Cmdline,
    ctx: &DetectionContext,
) -> (Option<ServiceNameSource>, Vec<String>) {
    let (Some(vendor), base_dir) = resolve_app_server(cmdline) else {
        return (None, vec![]);
    };

    // Resolve domain home path from base_dir
    let base_dir = PathBuf::from(base_dir);
    let domain_home = ctx
        .resolve_working_dir_relative_path(&base_dir)
        .unwrap_or(base_dir);

    let source = Some(vendor.as_source());

    // Find deployed applications
    let deployments = match find_deployed_apps(vendor, cmdline, ctx, &domain_home) {
        Ok(apps) => apps,
        Err(e) => {
            log::debug!(
                "extract_jee_service_names: error finding deployments: {}",
                e
            );
            return (source, vec![]);
        }
    };

    // Extract context roots from deployments
    let mut context_roots = Vec::new();
    for deployment in deployments {
        let roots = extract_context_roots_from_deployment(vendor, &deployment, ctx.fs);
        context_roots.extend(normalize_context_root(roots));
    }

    (source, context_roots)
}

/// Extracts context roots from a single deployment
fn extract_context_roots_from_deployment(
    vendor: ServerVendor,
    deployment: &Deployment,
    fs: &SubDirFs,
) -> Vec<String> {
    // If context root is already set, return it
    if let Some(ref cr) = deployment.context_root {
        return vec![cr.clone()];
    }

    // Determine deployment type from file extension if not already set
    let kind = deployment.kind.or_else(|| {
        let ext = Path::new(&deployment.name)
            .extension()
            .and_then(|s| s.to_str())
            .unwrap_or("")
            .to_lowercase();
        match ext.as_str() {
            "ear" => Some(DeploymentType::Ear),
            "war" => Some(DeploymentType::War),
            _ => None,
        }
    });

    // Create a filesystem abstraction from the deployment path
    let mut deployment_fs = match fs_from_deployment_path(fs, &deployment.path) {
        Ok(dfs) => dfs,
        Err(e) => {
            log::debug!(
                "extract_context_roots_from_deployment: failed to create deployment fs: {}",
                e
            );
            if kind == Some(DeploymentType::Ear) {
                return vec![];
            }
            return default_context_root_from_file(vendor, &deployment.name)
                .map(|cr| vec![cr])
                .unwrap_or_default();
        }
    };

    // Try to extract context root based on deployment type
    match kind {
        Some(DeploymentType::Ear) => {
            // For EARs, extract from application.xml
            extract_context_root_from_application_xml(&mut deployment_fs).unwrap_or_else(|e| {
                log::debug!(
                    "extract_context_roots_from_deployment: failed to extract from application.xml: {}",
                    e
                );
                vec![]
            })
        }
        Some(DeploymentType::War) => {
            // For WARs, try vendor-specific extraction first
            if let Some(cr) = custom_extract_war_context_root(vendor, &mut deployment_fs) {
                return vec![cr];
            }

            // Fall back to default extraction from filename
            default_context_root_from_file(vendor, &deployment.name)
                .map(|cr| vec![cr])
                .unwrap_or_default()
        }
        None => {
            // Unknown type, try default extraction from filename
            default_context_root_from_file(vendor, &deployment.name)
                .map(|cr| vec![cr])
                .unwrap_or_default()
        }
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::indexing_slicing)]
mod tests {
    use super::*;

    /// Type alias for error checking functions in tests
    pub(super) type ErrorChecker = Box<dyn Fn(&Error)>;

    #[test]
    fn test_extract_java_property() {
        let cmdline = crate::cmdline!["java", "-Dfoo=bar", "-Dbaz=qux", "-Dtest=value"];

        assert_eq!(
            extract_java_property_from_args(&cmdline, "-Dfoo="),
            Some("bar".to_string())
        );
        assert_eq!(
            extract_java_property_from_args(&cmdline, "-Dbaz="),
            Some("qux".to_string())
        );
        assert_eq!(
            extract_java_property_from_args(&cmdline, "-Dtest="),
            Some("value".to_string())
        );
        assert_eq!(
            extract_java_property_from_args(&cmdline, "-Dmissing="),
            None
        );
    }

    #[test]
    fn test_standard_extract_context_from_war_name() {
        assert_eq!(
            standard_extract_context_from_war_name("myapp.war"),
            Some("myapp".to_string())
        );
        assert_eq!(
            standard_extract_context_from_war_name("/path/to/myapp.war"),
            Some("myapp".to_string())
        );
        assert_eq!(
            standard_extract_context_from_war_name("myapp"),
            Some("myapp".to_string())
        );
    }

    #[test]
    fn test_normalize_context_root() {
        let roots = vec![
            "/test1".to_string(),
            "test2".to_string(),
            "/test3/test4".to_string(),
        ];
        let normalized = normalize_context_root(roots);
        assert_eq!(
            normalized,
            vec![
                "test1".to_string(),
                "test2".to_string(),
                "test3/test4".to_string()
            ]
        );
    }

    #[test]
    fn test_abs() {
        let base = Path::new("/base/path");
        assert_eq!(abs("relative", base), PathBuf::from("/base/path/relative"));
        assert_eq!(abs("/absolute", base), PathBuf::from("/absolute"));
    }

    // TestResolveAppServerFromCmdLine tests that vendor can be determined from the process cmdline
    #[test]
    fn test_resolve_app_server_from_cmdline() {
        struct TestCase {
            name: &'static str,
            raw_cmd: &'static str,
            expected_vendor: Option<ServerVendor>,
            expected_home: &'static str,
        }

        let tests = vec![
            TestCase {
                name: "wildfly 18 standalone",
                raw_cmd: r#"/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java -D[Standalone] -server
-Xms64m -Xmx512m -XX:MetaspaceSize=96M -XX:MaxMetaspaceSize=256m -Djava.net.preferIPv4Stack=true
-Djboss.modules.system.pkgs=org.jboss.byteman -Djava.awt.headless=true
--add-exports=java.base/sun.nio.ch=ALL-UNNAMED --add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED
--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED -Dorg.jboss.boot.log.file=/home/app/Downloads/wildfly-18.0.0.Final/standalone/log/server.log
-Dlogging.configuration=file:/home/app/Downloads/wildfly-18.0.0.Final/standalone/configuration/logging.properties
-jar /home/app/Downloads/wildfly-18.0.0.Final/jboss-modules.jar -mp /home/app/Downloads/wildfly-18.0.0.Final/modules org.jboss.as.standalone
-Djboss.home.dir=/home/app/Downloads/wildfly-18.0.0.Final -Djboss.server.base.dir=/home/app/Downloads/wildfly-18.0.0.Final/standalone"#,
                expected_vendor: Some(ServerVendor::JBoss),
                expected_home: "/home/app/Downloads/wildfly-18.0.0.Final/standalone",
            },
            TestCase {
                name: "wildfly 18 domain",
                raw_cmd: r#"/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java --add-exports=java.base/sun.nio.ch=ALL-UNNAMED
--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED --add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED -D[Server:server-one]
-D[pcid:780891833] -Xms64m -Xmx512m -server -XX:MetaspaceSize=96m -XX:MaxMetaspaceSize=256m -Djava.awt.headless=true -Djava.net.preferIPv4Stack=true
-Djboss.home.dir=/home/app/Downloads/wildfly-18.0.0.Final -Djboss.modules.system.pkgs=org.jboss.byteman
-Djboss.server.log.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/log
-Djboss.server.temp.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/tmp
-Djboss.server.data.dir=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/data
-Dorg.jboss.boot.log.file=/home/app/Downloads/wildfly-18.0.0.Final/domain/servers/server-one/log/server.log
-Dlogging.configuration=file:/home/app/Downloads/wildfly-18.0.0.Final/domain/configuration/default-server-logging.properties
-jar /home/app/Downloads/wildfly-18.0.0.Final/jboss-modules.jar -mp /home/app/Downloads/wildfly-18.0.0.Final/modules org.jboss.as.server"#,
                expected_vendor: Some(ServerVendor::JBoss),
                expected_home: "/home/app/Downloads/wildfly-18.0.0.Final/domain",
            },
            TestCase {
                name: "tomcat 10.x",
                raw_cmd: r#"java -Djava.util.logging.config.file=/app/Code/tomcat/apache-tomcat-10.0.27/conf/logging.properties
-Djava.util.logging.manager=org.apache.juli.ClassLoaderLogManager -Djdk.tls.ephemeralDHKeySize=2048
-Djava.protocol.handler.pkgs=org.apache.catalina.webresources -Dorg.apache.catalina.security.SecurityListener.UMASK=0027
-Dignore.endorsed.dirs= -classpath /app/Code/tomcat/apache-tomcat-10.0.27/bin/bootstrap.jar:/app/Code/tomcat/apache-tomcat-10.0.27/bin/tomcat-juli.jar
-Dcatalina.base=/app/Code/tomcat/apache-tomcat-10.0.27/myserver -Dcatalina.home=/app/Code/tomcat/apache-tomcat-10.0.27
-Djava.io.tmpdir=/app/Code/tomcat/apache-tomcat-10.0.27/temp org.apache.catalina.startup.Bootstrap start"#,
                expected_vendor: Some(ServerVendor::Tomcat),
                expected_home: "/app/Code/tomcat/apache-tomcat-10.0.27/myserver",
            },
            TestCase {
                name: "weblogic 12",
                raw_cmd: r#"/u01/jdk/bin/java -Djava.security.egd=file:/dev/./urandom -cp /u01/oracle/wlserver/server/lib/weblogic-launcher.jar
-Dlaunch.use.env.classpath=true -Dweblogic.Name=AdminServer -Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy
-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader -javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar
-da -Dwls.home=/u01/oracle/wlserver/server -Dweblogic.home=/u01/oracle/wlserver/server weblogic.Server"#,
                expected_vendor: Some(ServerVendor::WebLogic),
                expected_home: "",
            },
            TestCase {
                name: "websphere traditional 9.x",
                raw_cmd: r#"/opt/IBM/WebSphere/AppServer/java/8.0/bin/java -Dosgi.install.area=/opt/IBM/WebSphere/AppServer
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
/opt/IBM/WebSphere/AppServer/profiles/AppSrv01/config DefaultCell01 DefaultNode01 server1"#,
                expected_vendor: Some(ServerVendor::WebSphere),
                expected_home: "/opt/IBM/WebSphere/AppServer/profiles/AppSrv01",
            },
            TestCase {
                name: "weblogic deployer",
                raw_cmd: r#"/u01/jdk/bin/java -Djava.security.egd=file:/dev/./urandom -cp /u01/oracle/wlserver/server/lib/weblogic-launcher.jar
-Dlaunch.use.env.classpath=true -Dweblogic.Name=AdminServer -Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy
-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader -javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar
-da -Dwls.home=/u01/oracle/wlserver/server -Dweblogic.home=/u01/oracle/wlserver/server weblogic.Deployer -upload -target myserver -deploy some.war"#,
                expected_vendor: None,
                expected_home: "",
            },
        ];

        for tt in tests {
            let cmd_str = tt.raw_cmd.replace('\n', " ");
            let cmd_parts: Vec<&str> = cmd_str.split_whitespace().collect();
            let cmdline = Cmdline::from(&cmd_parts[..]);

            let (vendor, home) = resolve_app_server(&cmdline);

            assert_eq!(
                vendor, tt.expected_vendor,
                "{}: expected vendor {:?}, got {:?}",
                tt.name, tt.expected_vendor, vendor
            );

            // The base dir is making sense only when the vendor has been properly understood
            if tt.expected_vendor.is_some() {
                assert_eq!(
                    home, tt.expected_home,
                    "{}: expected home {}, got {}",
                    tt.name, tt.expected_home, home
                );
            }
        }
    }

    // TestExtractContextRootFromApplicationXml tests that context root can be extracted from an ear under /META-INF/application.xml
    #[test]
    fn test_extract_context_root_from_application_xml() {
        use std::fs;
        use tempfile::TempDir;

        struct TestCase {
            name: &'static str,
            xml: Option<&'static str>,
            expected: Option<Vec<&'static str>>,
        }

        let tests = vec![
            TestCase {
                name: "application.xml with webapps",
                xml: Some(
                    r#"<application xmlns="http://xmlns.jcp.org/xml/ns/javaee" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
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
</application>"#,
                ),
                expected: Some(vec!["MyWeb1", "MyWeb2"]),
            },
            TestCase {
                name: "application.xml with doctype and no webapps",
                xml: Some(
                    r#"<!DOCTYPE application PUBLIC "-//Sun Microsystems, Inc.//DTD J2EE Application 1.2//EN
http://java.sun.com/j2ee/dtds/application_1_2.dtd">
<application><module><java>my_app.jar</java></module></application>"#,
                ),
                expected: Some(vec![]),
            },
            TestCase {
                name: "no application.xml (invalid ear)",
                xml: None,
                expected: None,
            },
            TestCase {
                name: "invalid application.xml (invalid ear)",
                xml: Some("invalid"),
                expected: None,
            },
        ];

        for tt in tests {
            let tmp_dir = TempDir::new().unwrap();
            let ear_dir = tmp_dir.path().join("test.ear");
            let meta_inf = ear_dir.join("META-INF");

            // Create directory structure if we have XML content
            if let Some(xml_content) = tt.xml {
                fs::create_dir_all(&meta_inf).unwrap();
                fs::write(meta_inf.join("application.xml"), xml_content).unwrap();
            } else {
                // Create empty ear directory for the "no application.xml" case
                fs::create_dir_all(&ear_dir).unwrap();
            }

            let fs_root = SubDirFs::new(tmp_dir.path()).unwrap();
            let sub_fs = fs_root.sub(Path::new("test.ear")).unwrap();
            let mut deployment_fs = DeploymentFs::Directory(sub_fs);

            let result = extract_context_root_from_application_xml(&mut deployment_fs);

            match tt.expected {
                Some(expected) => {
                    assert!(
                        result.is_ok(),
                        "{}: expected success but got error: {:?}",
                        tt.name,
                        result.err()
                    );
                    let context_roots = result.unwrap();
                    assert_eq!(
                        context_roots.len(),
                        expected.len(),
                        "{}: expected {} context roots, got {}",
                        tt.name,
                        expected.len(),
                        context_roots.len()
                    );
                    for exp in expected {
                        assert!(
                            context_roots.contains(&exp.to_string()),
                            "{}: expected to find context root '{}' in {:?}",
                            tt.name,
                            exp,
                            context_roots
                        );
                    }
                }
                None => {
                    assert!(
                        result.is_err(),
                        "{}: expected error but got success: {:?}",
                        tt.name,
                        result.ok()
                    );
                }
            }
        }
    }

    /// test_weblogic_extract_service_names_for_jee_server tests all cases of
    /// detecting weblogic as vendor and extracting context root.
    /// It simulates having 1 ear deployed, 1 war with weblogic.xml and 1 war
    /// without weblogic.xml.
    ///
    /// Hence, it should extract ear context from application.xml, 1st war
    /// context from weblogic.xml and derive last war context from the filename.
    #[test]
    fn test_weblogic_extract_service_names_for_jee_server() {
        use std::collections::HashMap;
        use std::fs;
        use std::io::Write;
        use tempfile::TempDir;
        use zip::ZipWriter;
        use zip::write::SimpleFileOptions;

        let wls_config = r#"
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
</domain>"#;

        let app_xml = r#"
<application>
  <application-name>myapp</application-name>
  <initialize-in-order>false</initialize-in-order>
  <module>
	<web>
      <web-uri>app1.war</web-uri>
      <context-root>app1_context</context-root>
    </web>
  </module>
</application>"#;

        let weblogic_xml = r#"
<weblogic-web-app>
   <context-root>app2_context</context-root>
</weblogic-web-app>
"#;

        let tmp_dir = TempDir::new().unwrap();

        // Create directory structure
        let wls_domain = tmp_dir.path().join("wls/domain");
        let config_dir = wls_domain.join("config");
        let apps_dir = wls_domain.join("apps");
        fs::create_dir_all(&config_dir).unwrap();
        fs::create_dir_all(&apps_dir).unwrap();

        // Write config.xml
        fs::write(config_dir.join("config.xml"), wls_config).unwrap();

        // Create app1.ear with application.xml
        let app1_ear = apps_dir.join("app1.ear");
        let app1_meta_inf = app1_ear.join("META-INF");
        fs::create_dir_all(&app1_meta_inf).unwrap();
        fs::write(app1_meta_inf.join("application.xml"), app_xml).unwrap();

        // Create app2.war as a zip with weblogic.xml
        let app2_war_path = apps_dir.join("app2.war");
        let mut zip_buf = Vec::new();
        {
            let mut zip_writer = ZipWriter::new(std::io::Cursor::new(&mut zip_buf));
            zip_writer
                .start_file("META-INF/weblogic.xml", SimpleFileOptions::default())
                .unwrap();
            zip_writer.write_all(weblogic_xml.as_bytes()).unwrap();
            zip_writer.finish().unwrap();
        }
        fs::write(&app2_war_path, &zip_buf).unwrap();

        // Create app3.war as a directory (exploded WAR without weblogic.xml)
        let app3_war = apps_dir.join("app3.war");
        fs::create_dir_all(&app3_war).unwrap();

        // Setup filesystem and context
        let fs_root = SubDirFs::new(tmp_dir.path()).unwrap();
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "wls/domain".to_string());
        let ctx = DetectionContext::new(1, envs, &fs_root);

        // Simulate WebLogic command line args
        let cmdline = crate::cmdline![
            "java",
            "-Dweblogic.Name=AdminServer",
            "-Dwls.home=/wls",
            WLS_SERVER_MAIN_CLASS
        ];

        let (source, extracted_context_roots) = extract_names(&cmdline, &ctx);

        assert_eq!(
            source,
            Some(ServiceNameSource::Weblogic),
            "Expected WebLogic vendor"
        );
        assert_eq!(
            extracted_context_roots,
            vec![
                "app1_context".to_string(),
                "app2_context".to_string(),
                "app3".to_string()
            ]
        );
    }
}
