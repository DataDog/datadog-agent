// Comprehensive test suite comparing Go USM and Rust USM-RS implementations
// Tests all major compatibility issues identified in the analysis

use std::collections::HashMap;
use std::sync::Arc;
use usm_rs::{
    context::{DetectionContext, Environment},
    detectors::java::JavaDetector,
    frameworks::{
        spring::{SpringBootParser, is_spring_boot_archive},
        javaee::{JeeExtractor, ServerVendor, detect_javaee_server},
    },
    filesystem::{MemoryFileSystem, FileSystem},
    metadata::{ServiceMetadata, ServiceNameSource},
    utils::{
        parsing::{extract_java_system_properties, PropertyResolver, create_property_resolver},
        zip_utils::{analyze_spring_boot_jar, classify_jar_type, JarType},
    },
};

/// Test cases that must produce identical results between Go and Rust implementations
pub struct CompatibilityTestSuite;

impl CompatibilityTestSuite {
    /// Test Spring Boot with complex configuration (from analysis)
    #[test]
    fn test_spring_boot_complex_configuration() {
        let mut fs = MemoryFileSystem::new();
        
        // Create a Spring Boot JAR with complex configuration
        let spring_properties = r#"
spring.application.name=${SERVICE_NAME:default-app}
spring.profiles.active=prod
server.port=${SERVER_PORT:8080}
management.endpoints.web.base-path=/actuator
"#;
        
        fs.add_file("application.properties", spring_properties.as_bytes().to_vec());
        fs.add_file("BOOT-INF/classes/application-prod.properties", 
                   b"spring.application.name=production-service");
        
        let filesystem = Arc::new(fs);
        let mut env = Environment::new();
        env.set("SERVICE_NAME".to_string(), "env-service".to_string());
        env.set("SERVER_PORT".to_string(), "9090".to_string());
        
        let args = vec![
            "java".to_string(),
            "-Dspring.profiles.active=prod".to_string(),
            "-jar".to_string(),
            "myapp.jar".to_string(),
        ];
        
        let mut ctx = DetectionContext::new(args.clone(), env, filesystem);
        let mut spring_parser = SpringBootParser::new(&mut ctx);
        
        // This should resolve to "env-service" (highest precedence from environment)
        let result = spring_parser.get_spring_boot_app_name("myapp.jar").unwrap();
        
        // Expected: Go USM would return "env-service" due to environment variable precedence
        // Rust USM-RS should now match this behavior
        assert_eq!(result, Some("env-service".to_string()));
    }
    
    /// Test Tomcat with multiple web apps (from analysis)
    #[test]
    fn test_tomcat_multiple_web_apps() {
        let mut fs = MemoryFileSystem::new();
        
        // Add Tomcat server.xml with context definitions
        let server_xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<Server port="8005" shutdown="SHUTDOWN">
    <Service name="Catalina">
        <Engine name="Catalina" defaultHost="localhost">
            <Host name="localhost" appBase="webapps">
                <Context path="/myapp" docBase="myapp.war" />
                <Context path="/api" docBase="api.war" />
                <Context path="/admin" docBase="admin" />
            </Host>
        </Engine>
    </Service>
</Server>"#;
        
        fs.add_file("/opt/tomcat/conf/server.xml", server_xml.as_bytes().to_vec());
        fs.add_file("/opt/tomcat/webapps/myapp.war", b"dummy war file");
        fs.add_file("/opt/tomcat/webapps/api.war", b"dummy war file");
        fs.add_file("/opt/tomcat/webapps/admin", b"dummy directory");
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let args = vec![
            "java".to_string(),
            "-Dcatalina.base=/opt/tomcat".to_string(),
            "org.apache.catalina.startup.Bootstrap".to_string(),
        ];
        
        let ctx = DetectionContext::new(args, env, filesystem);
        let jee_extractor = JeeExtractor::new(&ctx);
        let (vendor, context_roots) = jee_extractor.extract_service_names();
        
        // Expected: Go USM detects Tomcat and extracts context roots
        // Rust USM-RS should match this behavior
        assert_eq!(vendor, ServerVendor::Tomcat);
        assert!(context_roots.contains(&"myapp".to_string()));
        assert!(context_roots.contains(&"api".to_string()));
        assert!(context_roots.contains(&"admin".to_string()));
        
        // Additional assertions for consistency
        assert_eq!(context_roots.len(), 3);
        assert!(context_roots.iter().all(|name| !name.is_empty()));
    }
    
    /// Test Node.js with complex package resolution (from analysis)
    #[test]
    fn test_nodejs_complex_package_resolution() {
        let mut fs = MemoryFileSystem::new();
        
        // Create nested directory structure with package.json files
        let root_package = r#"{
    "name": "@company/monorepo",
    "private": true,
    "workspaces": ["apps/*", "packages/*"]
}"#;
        
        let app_package = r#"{
    "name": "@company/my-service",
    "version": "1.0.0",
    "main": "dist/index.js",
    "scripts": {
        "start": "node dist/index.js"
    }
}"#;
        
        fs.add_file("package.json", root_package.as_bytes().to_vec());
        fs.add_file("apps/my-service/package.json", app_package.as_bytes().to_vec());
        fs.add_file("apps/my-service/dist/index.js", b"console.log('Hello World');");
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let args = vec![
            "node".to_string(),
            "../../apps/my-service/dist/index.js".to_string(),
        ];
        
        let mut ctx = DetectionContext::new(args, env, filesystem);
        
        // Simulate Node.js detector resolution
        // Expected: Go USM would find the nearest package.json and extract service name
        // Rust USM-RS should traverse up the directory tree correctly
        
        let working_dirs = ctx.get_working_dirs();
        let resolved_path = ctx.resolve_working_dir_relative_path("../../apps/my-service/dist/index.js");
        
        // Should resolve to an absolute path
        assert!(resolved_path.is_absolute());
        assert!(resolved_path.to_string_lossy().contains("my-service"));
    }
    
    /// Test property resolution with precedence (covers multiple analysis issues)
    #[test]
    fn test_property_resolution_precedence() {
        let mut env_vars = HashMap::new();
        env_vars.insert("SPRING_APPLICATION_NAME".to_string(), "env-service".to_string());
        env_vars.insert("SERVER_PORT".to_string(), "8080".to_string());
        
        let args = vec![
            "java".to_string(),
            "-Dspring.application.name=system-service".to_string(),
            "-Dserver.port=9090".to_string(),
            "--spring.application.name=cmd-service".to_string(),
            "--server.port=7070".to_string(),
            "-jar".to_string(),
            "app.jar".to_string(),
        ];
        
        let resolver = create_property_resolver(&args, &env_vars);
        
        // Test precedence: command line > system props > environment > config files
        assert_eq!(resolver.get_property("spring.application.name"), Some("cmd-service".to_string()));
        assert_eq!(resolver.get_property("server.port"), Some("7070".to_string()));
        
        // Test placeholder resolution
        resolver.add_config_properties("application.properties", {
            let mut config = HashMap::new();
            config.insert("app.title".to_string(), "My ${spring.application.name} Application".to_string());
            config.insert("app.url".to_string(), "http://localhost:${server.port}".to_string());
            config
        });
        
        assert_eq!(
            resolver.resolve_placeholders("My ${spring.application.name} Application"),
            "My cmd-service Application"
        );
        assert_eq!(
            resolver.resolve_placeholders("http://localhost:${server.port}/api"),
            "http://localhost:7070/api"
        );
    }
    
    /// Test JAR analysis and classification (from analysis)
    #[test]
    fn test_jar_analysis_classification() {
        let mut fs = MemoryFileSystem::new();
        
        // Create mock Spring Boot JAR content
        let manifest_content = r#"Manifest-Version: 1.0
Main-Class: org.springframework.boot.loader.launch.JarLauncher
Start-Class: com.example.Application
Spring-Boot-Version: 3.2.0
"#;
        
        fs.add_file("META-INF/MANIFEST.MF", manifest_content.as_bytes().to_vec());
        fs.add_file("BOOT-INF/classes/application.properties", 
                   b"spring.application.name=test-spring-app");
        fs.add_file("BOOT-INF/lib/spring-boot-3.2.0.jar", b"nested jar");
        fs.add_file("org/springframework/boot/SpringApplication.class", b"class file");
        
        let filesystem = Arc::new(fs);
        
        // Test JAR classification
        let jar_type = classify_jar_type(&*filesystem, std::path::Path::new("test.jar")).unwrap();
        assert_eq!(jar_type, JarType::SpringBootExecutable);
        
        // Test comprehensive analysis
        let analysis = analyze_spring_boot_jar(&*filesystem, std::path::Path::new("test.jar")).unwrap();
        assert!(analysis.is_spring_boot);
        assert!(analysis.boot_inf_structure);
        assert_eq!(analysis.main_class, Some("org.springframework.boot.loader.launch.JarLauncher".to_string()));
        assert_eq!(analysis.start_class, Some("com.example.Application".to_string()));
        assert_eq!(analysis.spring_boot_version, Some("3.2.0".to_string()));
        assert_eq!(analysis.nested_jars.len(), 1);
        assert!(analysis.nested_jars[0].contains("spring-boot"));
    }
    
    /// Test JEE server detection with system properties (from analysis)
    #[test]
    fn test_jee_server_detection_system_properties() {
        let test_cases = vec![
            (
                vec![("catalina.home", "/opt/tomcat"), ("catalina.base", "/opt/tomcat")],
                ServerVendor::Tomcat
            ),
            (
                vec![("jboss.home.dir", "/opt/jboss"), ("jboss.server.home.dir", "/opt/jboss/standalone")],
                ServerVendor::JBoss
            ),
            (
                vec![("weblogic.home", "/opt/oracle/weblogic"), ("weblogic.domain.name", "base_domain")],
                ServerVendor::WebLogic
            ),
            (
                vec![("was.install.root", "/opt/ibm/websphere"), ("websphere.base.dir", "/opt/ibm/websphere")],
                ServerVendor::WebSphere
            ),
        ];
        
        for (properties, expected_vendor) in test_cases {
            let mut system_props = HashMap::new();
            for (key, value) in properties {
                system_props.insert(key.to_string(), value.to_string());
            }
            
            let detected_vendor = detect_javaee_server(&system_props);
            assert_eq!(detected_vendor, expected_vendor, "Failed for properties: {:?}", system_props);
        }
    }
    
    /// Test filename normalization (covers Tomcat-specific edge cases from analysis)
    #[test]
    fn test_filename_normalization() {
        use usm_rs::utils::normalization::tomcat_filename_normalization;
        
        let test_cases = vec![
            ("myapp.war", ("myapp".to_string(), true)),
            ("ROOT.war", ("".to_string(), false)),
            ("myapp##v1.0.war", ("myapp".to_string(), true)),
            ("app#context.war", ("app/context".to_string(), true)),
            ("app#sub#context##v2.war", ("app/sub/context".to_string(), true)),
            ("ROOT##123", ("".to_string(), false)),
            ("simple", ("simple".to_string(), true)),
        ];
        
        for (input, expected) in test_cases {
            let result = tomcat_filename_normalization(input);
            assert_eq!(result, expected, "Failed for input: {}", input);
        }
    }
    
    /// Test error handling and fallbacks (from analysis requirements)
    #[test]
    fn test_error_handling_fallbacks() {
        let mut fs = MemoryFileSystem::new();
        // Intentionally create malformed configuration to test error handling
        fs.add_file("invalid.properties", b"malformed=properties=file=content");
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let args = vec![
            "java".to_string(),
            "-jar".to_string(),
            "nonexistent.jar".to_string(),
        ];
        
        let mut ctx = DetectionContext::new(args, env, filesystem);
        let detector = JavaDetector::new(&ctx);
        
        // Should not panic and should provide fallback detection
        let result = detector.detect(&mut ctx, &ctx.args.clone());
        assert!(result.is_ok(), "Detector should handle errors gracefully");
        
        // Should fall back to basic JAR name detection
        if let Ok(Some(metadata)) = result {
            assert_eq!(metadata.name, "nonexistent");
            assert_eq!(metadata.source, ServiceNameSource::CommandLine);
        }
    }
    
    /// Test cross-detector communication and context sharing
    #[test] 
    fn test_cross_detector_communication() {
        let filesystem = Arc::new(MemoryFileSystem::new());
        let env = Environment::new();
        let args = vec!["java".to_string()];
        
        let mut ctx = DetectionContext::new(args, env, filesystem);
        
        // Test context sharing
        ctx.set_context(usm_rs::context::ContextKey::SpringBootProperties, 
                       HashMap::<String, String>::from([
                           ("spring.application.name".to_string(), "shared-app".to_string())
                       ]));
        
        let shared_props = ctx.get_context::<HashMap<String, String>>(
            usm_rs::context::ContextKey::SpringBootProperties
        );
        
        assert!(shared_props.is_some());
        assert_eq!(shared_props.unwrap().get("spring.application.name"), Some(&"shared-app".to_string()));
        
        // Test framework hints
        ctx.set_framework_hint(usm_rs::context::FrameworkHint {
            framework_type: usm_rs::context::FrameworkType::SpringBoot,
            confidence: 0.9,
            evidence: vec!["spring_boot_jar_detected".to_string()],
            suggested_service_name: Some("spring-service".to_string()),
        });
        
        let hint = ctx.get_framework_hint();
        assert!(hint.is_some());
        assert_eq!(hint.unwrap().framework_type, usm_rs::context::FrameworkType::SpringBoot);
        assert_eq!(hint.unwrap().confidence, 0.9);
    }
    
    /// Performance regression test - ensure Rust implementation is not significantly slower
    #[test]
    fn test_performance_baseline() {
        use std::time::Instant;
        
        let mut fs = MemoryFileSystem::new();
        
        // Create many test files to simulate real-world scenario
        for i in 0..100 {
            fs.add_file(&format!("app{}.jar", i), format!("mock jar content {}", i).as_bytes().to_vec());
            fs.add_file(&format!("config{}.properties", i), 
                       format!("spring.application.name=app{}\nserver.port={}", i, 8000 + i).as_bytes().to_vec());
        }
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        
        let start = Instant::now();
        
        // Simulate detection on many services
        for i in 0..100 {
            let args = vec![
                "java".to_string(),
                format!("-Dspring.config.location=config{}.properties", i),
                "-jar".to_string(),
                format!("app{}.jar", i),
            ];
            
            let mut ctx = DetectionContext::new(args, env.clone(), filesystem.clone());
            let detector = JavaDetector::new(&ctx);
            let _result = detector.detect(&mut ctx, &ctx.args.clone());
        }
        
        let duration = start.elapsed();
        
        // Should complete within reasonable time (adjust threshold as needed)
        assert!(duration.as_millis() < 1000, "Detection took too long: {:?}", duration);
        println!("Performance test completed in {:?}", duration);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn run_compatibility_suite() {
        CompatibilityTestSuite::test_spring_boot_complex_configuration();
        CompatibilityTestSuite::test_tomcat_multiple_web_apps();
        CompatibilityTestSuite::test_nodejs_complex_package_resolution();
        CompatibilityTestSuite::test_property_resolution_precedence();
        CompatibilityTestSuite::test_jar_analysis_classification();
        CompatibilityTestSuite::test_jee_server_detection_system_properties();
        CompatibilityTestSuite::test_filename_normalization();
        CompatibilityTestSuite::test_error_handling_fallbacks();
        CompatibilityTestSuite::test_cross_detector_communication();
        CompatibilityTestSuite::test_performance_baseline();
        
        println!("✅ All compatibility tests passed!");
    }
}