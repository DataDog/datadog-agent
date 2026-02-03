// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use crate::service_name::{DetectionContext, ServiceNameMetadata, ServiceNameSource};
use std::path::Path;

// Constants from the Go implementation
const JAR_EXTENSION: &str = ".jar";
const WAR_EXTENSION: &str = ".war";
const APACHE_PREFIX: &str = "org.apache.";
const SPRING_BOOT_LAUNCHER: &str = "org.springframework.boot.loader.launch.JarLauncher";
const SPRING_BOOT_OLD_LAUNCHER: &str = "org.springframework.boot.loader.JarLauncher";

/// Checks if an argument is a Java name flag (-jar, -m, or --module)
fn is_name_flag(arg: &str) -> bool {
    matches!(arg, "-jar" | "-m" | "--module")
}

/// Removes the file path from a string, keeping only the base name
fn remove_file_path(s: &str) -> &str {
    Path::new(s)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(s)
}

/// Trims everything after (and including) the first colon.  If there is no
/// color or no content before the colon, return the original string.
fn trim_colon_right(s: &str) -> &str {
    let Some((prefix, _)) = s.split_once(':') else {
        return s;
    };

    if prefix.is_empty() {
        return s;
    }

    prefix
}

/// Extracts the Java service name from the command line
pub fn extract_name(
    cmdline: &crate::procfs::Cmdline,
    ctx: &mut DetectionContext,
) -> Option<ServiceNameMetadata> {
    // First pass: Look for -Ddd.service= system property (highest priority)
    let mut args_iter = cmdline.args();
    args_iter.next()?; // Skip the java executable

    if let Some(service_name) = args_iter
        .find_map(|arg| arg.strip_prefix("-Ddd.service="))
        .filter(|name| !name.is_empty())
    {
        return Some(ServiceNameMetadata::new(
            service_name,
            ServiceNameSource::CommandLine,
        ));
    }

    // Second pass: Look for service name based on command line structure
    let mut args_iter = cmdline.args();
    args_iter.next()?; // Skip the java executable again

    let mut prev_arg_is_flag = false;

    for a in args_iter {
        let has_flag_prefix = a.starts_with('-');
        let includes_assignment = a.contains('=')
            || a.starts_with("-X")
            || a.starts_with("-javaagent:")
            || a.starts_with("-verbose:");
        // @ is used to point to a file with more arguments. We do not support
        // it at the moment, so explicitly ignore it to avoid naming services
        // based on this file name.
        let at_arg = a.starts_with('@');
        let should_skip_arg = prev_arg_is_flag || has_flag_prefix || includes_assignment || at_arg;

        if !should_skip_arg {
            let mut arg = remove_file_path(a);
            arg = trim_colon_right(arg);

            if arg.starts_with(|c: char| c.is_alphabetic()) {
                // Do JEE detection to see if we can extract additional service names from context roots
                let (vendor_source, additional_names) = super::jee::extract_names(cmdline, ctx);

                // The name gets joined to the AdditionalNames, so a part of
                // the name still comes from the command line, but report
                // the source as the web server since that is not easy to
                // guess from looking at the command line.
                let source = if additional_names.is_empty() {
                    ServiceNameSource::CommandLine
                } else {
                    vendor_source.unwrap_or(ServiceNameSource::CommandLine)
                };

                // Check for JAR or WAR files
                if arg.ends_with(JAR_EXTENSION) || arg.ends_with(WAR_EXTENSION) {
                    // Try to see if the application is a Spring Boot archive and extract its application name
                    if additional_names.is_empty()
                        && let Some(spring_app_name) =
                            super::spring::get_spring_boot_app_name(a, ctx, cmdline)
                    {
                        return Some(ServiceNameMetadata::new(
                            spring_app_name,
                            ServiceNameSource::Spring,
                        ));
                    }

                    // Strip the .jar or .war extension
                    let name = arg.get(..arg.len() - JAR_EXTENSION.len()).unwrap_or(arg);
                    return Some(
                        ServiceNameMetadata::new(name.to_string(), source)
                            .with_additional_names(additional_names),
                    );
                }

                // Check for org.apache.* classes
                if let Some(name) = arg.strip_prefix(APACHE_PREFIX).and_then(|rest| {
                    // Take the project name after the package 'org.apache.' while stripping off
                    // the remaining package and class name
                    rest.split_once('.').map(|(name, _)| name.to_string())
                }) {
                    return Some(
                        ServiceNameMetadata::new(name, source)
                            .with_additional_names(additional_names),
                    );
                }

                // Check for Spring Boot launcher classes
                if (arg == SPRING_BOOT_LAUNCHER || arg == SPRING_BOOT_OLD_LAUNCHER)
                    && let Some(spring_app_name) =
                        super::spring::get_spring_boot_launcher_app_name(ctx, cmdline)
                {
                    return Some(ServiceNameMetadata::new(
                        spring_app_name,
                        ServiceNameSource::Spring,
                    ));
                }

                // Default: use the class name or JAR name as-is
                return Some(
                    ServiceNameMetadata::new(arg.to_string(), source)
                        .with_additional_names(additional_names),
                );
            }
        }

        prev_arg_is_flag = has_flag_prefix && !includes_assignment && !is_name_flag(a);
    }

    None
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::fs::SubDirFs;
    use crate::procfs::Cmdline;
    use crate::test_utils::TestDataFs;
    use std::collections::HashMap;

    fn test_ctx() -> (HashMap<String, String>, SubDirFs) {
        let tempdir = std::env::temp_dir();
        let fs = SubDirFs::new(&tempdir).unwrap();
        let envs = HashMap::new();
        (envs, fs)
    }

    #[test]
    fn test_java_jar_flag() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Xmx4000m",
            "-Xms4000m",
            "-XX:ReservedCodeCacheSize=256m",
            "-jar",
            "/opt/sheepdog/bin/myservice.jar"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myservice");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
        assert!(metadata.additional_names.is_empty());
    }

    #[test]
    fn test_java_war_file() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Duser.home=/var/jenkins_home",
            "-Dhudson.lifecycle=hudson.lifecycle.ExitLifecycle",
            "-jar",
            "/usr/share/jenkins/jenkins.war",
            "--httpPort=8000"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "jenkins");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_class_name() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Xmx4000m",
            "-Xms4000m",
            "-XX:ReservedCodeCacheSize=256m",
            "com.datadog.example.HelloWorld"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "com.datadog.example.HelloWorld");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_module_flag() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Xmx4000m",
            "-Xms4000m",
            "-XX:ReservedCodeCacheSize=256m",
            "-m",
            "org.elasticsearch.server/org.elasticsearch.bootstrap.Elasticsearch"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        // The module flag extracts the class name after the '/'
        assert_eq!(metadata.name, "org.elasticsearch.bootstrap.Elasticsearch");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_module_long_flag() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Xmx4000m",
            "--module",
            "org.elasticsearch.server/org.elasticsearch.bootstrap.Elasticsearch",
            "-Xfoo"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        // The module flag extracts the class name after the '/'
        assert_eq!(metadata.name, "org.elasticsearch.bootstrap.Elasticsearch");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_module_flag_after_class() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["java", "foo", "-m", "argument-to-app"];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "foo");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_ignore_at_file() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["java", "@/tmp/foo21321312.tmp"];

        let result = extract_name(&cmdline, &mut ctx);

        // Should return None since @ file is ignored and there's no valid class/jar name
        assert!(result.is_none());
    }

    #[test]
    fn test_java_ignore_at_file_with_app() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["java", "@foo.extra", "myapp"];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_kafka() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "java",
            "-Xmx4000m",
            "-Xms4000m",
            "-XX:ReservedCodeCacheSize=256m",
            "kafka.Kafka"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "kafka.Kafka");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_apache_cassandra() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![
            "/usr/bin/java",
            "-Xloggc:/usr/share/cassandra/logs/gc.log",
            "-ea",
            "-XX:+HeapDumpOnOutOfMemoryError",
            "-Xss256k",
            "-Dlogback.configurationFile=logback.xml",
            "-Dcassandra.logdir=/var/log/cassandra",
            "-Dcassandra.storagedir=/data/cassandra",
            "-cp",
            "/etc/cassandra:/usr/share/cassandra/lib/HdrHistogram-2.1.9.jar",
            "org.apache.cassandra.service.CassandraDaemon"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "cassandra");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_space_in_path() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["/home/dd/my java dir/java", "com.dog.cat"];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "com.dog.cat");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_dd_service_property() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["/usr/bin/java", "-Ddd.service=custom", "-jar", "app.jar"];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "custom");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_dd_service_empty() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["/usr/bin/java", "-Ddd.service=", "-jar", "app.jar"];

        let result = extract_name(&cmdline, &mut ctx);

        // Empty dd.service should be ignored, should fall back to jar name
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_is_name_flag() {
        assert!(is_name_flag("-jar"));
        assert!(is_name_flag("-m"));
        assert!(is_name_flag("--module"));
        assert!(!is_name_flag("-cp"));
        assert!(!is_name_flag("--classpath"));
        assert!(!is_name_flag("-Xmx"));
    }

    #[test]
    fn test_remove_file_path() {
        assert_eq!(remove_file_path("/opt/app/myservice.jar"), "myservice.jar");
        assert_eq!(remove_file_path("myservice.jar"), "myservice.jar");
        assert_eq!(remove_file_path("/usr/bin/java"), "java");
        assert_eq!(remove_file_path(""), "");
    }

    #[test]
    fn test_trim_colon_right() {
        assert_eq!(trim_colon_right("foo:bar"), "foo");
        assert_eq!(trim_colon_right("foo"), "foo");
        assert_eq!(trim_colon_right("foo:bar:baz"), "foo");
        assert_eq!(trim_colon_right(":foo"), ":foo"); // idx must be > 0
        assert_eq!(trim_colon_right(""), "");
    }

    #[test]
    fn test_spring_boot_unpacked_jar_with_new_launcher() {
        let fs = TestDataFs::new("spring");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "with-prop".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["java", "-jar", SPRING_BOOT_LAUNCHER];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "spring-boot-app-name");
        assert_eq!(metadata.source, ServiceNameSource::Spring);
    }

    #[test]
    fn test_spring_boot_unpacked_jar_with_classpath() {
        let fs = TestDataFs::new("spring");
        let envs = HashMap::new();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["java", "-cp", "with-prop:foo", "-jar", SPRING_BOOT_LAUNCHER];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "spring-boot-app-name");
        assert_eq!(metadata.source, ServiceNameSource::Spring);
    }

    #[test]
    fn test_spring_boot_unpacked_jar_with_old_launcher() {
        let fs = TestDataFs::new("spring");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "with-prop".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["java", "-jar", SPRING_BOOT_OLD_LAUNCHER];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "spring-boot-app-name");
        assert_eq!(metadata.source, ServiceNameSource::Spring);
    }

    #[test]
    fn test_spring_boot_default_options() {
        use std::io::Write;
        use tempfile::TempDir;

        // Create a temporary Spring Boot JAR
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = tmp_dir.path().join("app").join("app.jar");
        std::fs::create_dir_all(jar_path.parent().unwrap()).unwrap();

        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);

        writer.start_file("BOOT-INF/", options).unwrap();
        writer
            .start_file("BOOT-INF/classes/application.properties", options)
            .unwrap();
        writer
            .write_all(b"spring.application.name=default-app")
            .unwrap();
        writer
            .start_file(
                "BOOT-INF/classes/config/prod/application-prod.properties",
                options,
            )
            .unwrap();
        writer
            .write_all(b"spring.application.name=prod-app")
            .unwrap();
        writer.finish().unwrap();

        let jar_path_str = jar_path.to_string_lossy().to_string();
        let fs = SubDirFs::new("/").unwrap();
        let envs = HashMap::new();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["java", "-jar", &jar_path_str];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "default-app");
        assert_eq!(metadata.source, ServiceNameSource::Spring);
        assert!(metadata.additional_names.is_empty());
    }

    // JEE Integration Tests
    const JBOSS_TEST_APP_ROOT: &str = "../sub";

    #[test]
    fn test_wildfly_18_standalone() {
        let fs = TestDataFs::new("jee/jboss");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/sibling".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());

        let log_file = format!(
            "-Dorg.jboss.boot.log.file={}/standalone/log/server.log",
            JBOSS_TEST_APP_ROOT
        );
        let logging_config = format!(
            "-Dlogging.configuration=file:{}/standalone/configuration/logging.properties",
            JBOSS_TEST_APP_ROOT
        );
        let jar_path = format!("{}/jboss-modules.jar", JBOSS_TEST_APP_ROOT);
        let modules_path = format!("{}/modules", JBOSS_TEST_APP_ROOT);
        let home_dir = format!("-Djboss.home.dir={}", JBOSS_TEST_APP_ROOT);
        let base_dir = format!("-Djboss.server.base.dir={}/standalone", JBOSS_TEST_APP_ROOT);

        let cmdline = cmdline![
            "home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java",
            "-D[Standalone]",
            "-server",
            "-Xms64m",
            "-Xmx512m",
            "-XX:MetaspaceSize=96M",
            "-XX:MaxMetaspaceSize=256m",
            "-Djava.net.preferIPv4Stack=true",
            "-Djboss.modules.system.pkgs=org.jboss.byteman",
            "-Djava.awt.headless=true",
            "--add-exports=java.base/sun.nio.ch=ALL-UNNAMED",
            "--add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED",
            "--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED",
            &log_file,
            &logging_config,
            "-jar",
            &jar_path,
            "-mp",
            &modules_path,
            "org.jboss.as.standalone",
            &home_dir,
            &base_dir
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "jboss-modules");
        assert_eq!(metadata.source, ServiceNameSource::Jboss);
        assert_eq!(
            metadata.additional_names,
            vec!["my-jboss-webapp", "some_context_root", "web3"]
        );
    }

    #[test]
    fn test_wildfly_18_domain() {
        let fs = TestDataFs::new("jee/jboss");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/sibling".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());

        let home_dir = format!("-Djboss.home.dir={}", JBOSS_TEST_APP_ROOT);
        let log_dir = format!(
            "-Djboss.server.log.dir={}/domain/servers/server-one/log",
            JBOSS_TEST_APP_ROOT
        );
        let temp_dir = format!(
            "-Djboss.server.temp.dir={}/domain/servers/server-one/tmp",
            JBOSS_TEST_APP_ROOT
        );
        let data_dir = format!(
            "-Djboss.server.data.dir={}/domain/servers/server-one/data",
            JBOSS_TEST_APP_ROOT
        );
        let log_file = format!(
            "-Dorg.jboss.boot.log.file={}/domain/servers/server-one/log/server.log",
            JBOSS_TEST_APP_ROOT
        );
        let logging_config = format!(
            "-Dlogging.configuration=file:{}/domain/configuration/default-server-logging.properties",
            JBOSS_TEST_APP_ROOT
        );
        let jar_path = format!("{}/jboss-modules.jar", JBOSS_TEST_APP_ROOT);
        let modules_path = format!("{}/modules", JBOSS_TEST_APP_ROOT);

        let cmdline = cmdline![
            "/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java",
            "--add-exports=java.base/sun.nio.ch=ALL-UNNAMED",
            "--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED",
            "--add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED",
            "-D[Server:server-one]",
            "-D[pcid:780891833]",
            "-Xms64m",
            "-Xmx512m",
            "-server",
            "-XX:MetaspaceSize=96m",
            "-XX:MaxMetaspaceSize=256m",
            "-Djava.awt.headless=true",
            "-Djava.net.preferIPv4Stack=true",
            &home_dir,
            "-Djboss.modules.system.pkgs=org.jboss.byteman",
            &log_dir,
            &temp_dir,
            &data_dir,
            &log_file,
            &logging_config,
            "-jar",
            &jar_path,
            "-mp",
            &modules_path,
            "org.jboss.as.server"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "jboss-modules");
        assert_eq!(metadata.source, ServiceNameSource::Jboss);
        assert_eq!(metadata.additional_names, vec!["web3", "web4"]);
    }

    #[test]
    fn test_weblogic_12() {
        let fs = TestDataFs::new("jee/weblogic");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/sub".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());

        let cmdline = cmdline![
            "/u01/jdk/bin/java",
            "-Djava.security.egd=file:/dev/./urandom",
            "-cp",
            "/u01/oracle/wlserver/server/lib/weblogic-launcher.jar",
            "-Dlaunch.use.env.classpath=true",
            "-Dweblogic.Name=AdminServer",
            "-Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy",
            "-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader",
            "-javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar",
            "-da",
            "-Dwls.home=/u01/oracle/wlserver/server",
            "-Dweblogic.home=/u01/oracle/wlserver/server",
            "weblogic.Server"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "weblogic.Server");
        assert_eq!(metadata.source, ServiceNameSource::Weblogic);
        assert_eq!(
            metadata.additional_names,
            vec!["my_context", "sample4", "some_context_root"]
        );
    }

    #[test]
    fn test_tomcat_10() {
        let fs = TestDataFs::new("jee");
        let envs = HashMap::new();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());

        let cmdline = cmdline![
            "/usr/bin/java",
            "-Djava.util.logging.config.file=tomcat/conf/logging.properties",
            "-Djava.util.logging.manager=org.apache.juli.ClassLoaderLogManager",
            "-Djdk.tls.ephemeralDHKeySize=2048",
            "-Djava.protocol.handler.pkgs=org.apache.catalina.webresources",
            "-Dorg.apache.catalina.security.SecurityListener.UMASK=0027",
            "--add-opens=java.base/java.lang=ALL-UNNAMED",
            "--add-opens=java.base/java.io=ALL-UNNAMED",
            "--add-opens=java.base/java.util=ALL-UNNAMED",
            "--add-opens=java.base/java.util.concurrent=ALL-UNNAMED",
            "--add-opens=java.rmi/sun.rmi.transport=ALL-UNNAMED",
            "-classpath",
            "tomcat/bin/bootstrap.jar:tomcat/bin/tomcat-juli.jar",
            "-Dcatalina.base=tomcat",
            "-Dcatalina.home=tomcat",
            "-Djava.io.tmpdir=tomcat/temp",
            "org.apache.catalina.startup.Bootstrap",
            "start"
        ];

        let result = extract_name(&cmdline, &mut ctx);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "catalina");
        assert_eq!(metadata.source, ServiceNameSource::Tomcat);
        assert_eq!(metadata.additional_names, vec!["app2", "custom"]);
    }
}
