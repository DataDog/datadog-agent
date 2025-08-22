use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::frameworks::spring::SpringBootParser;
use crate::frameworks::javaee::{JeeExtractor, ServerVendor};
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::path_utils::{remove_file_path, trim_colon_right, is_rune_letter_at};

const JAVA_JAR_EXTENSION: &str = ".jar";
const JAVA_WAR_EXTENSION: &str = ".war";
const JAVA_APACHE_PREFIX: &str = "org.apache.";
const SPRING_BOOT_LAUNCHER: &str = "org.springframework.boot.loader.launch.JarLauncher";
const SPRING_BOOT_OLD_LAUNCHER: &str = "org.springframework.boot.loader.JarLauncher";

/// Java detector for JAR, WAR, and Java application detection
pub struct JavaDetector;

impl JavaDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(JavaDetector)
    }
    
    /// Check if an argument is a Java name flag (-jar, -m, --module)
    fn is_name_flag(arg: &str) -> bool {
        arg == "-jar" || arg == "-m" || arg == "--module"
    }
    
    /// Get the vendor-to-source mapping
    fn get_vendor_source(vendor: ServerVendor) -> ServiceNameSource {
        match vendor {
            ServerVendor::Tomcat => ServiceNameSource::Tomcat,
            ServerVendor::WebLogic => ServiceNameSource::WebLogic,
            ServerVendor::WebSphere => ServiceNameSource::WebSphere,
            ServerVendor::JBoss => ServiceNameSource::JBoss,
            _ => ServiceNameSource::CommandLine,
        }
    }
}

impl Detector for JavaDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        let mut metadata = ServiceMetadata::new(String::new(), ServiceNameSource::CommandLine);
        let mut fallback_metadata: Option<ServiceMetadata> = None;
        
        // Look for dd.service system property
        for arg in args {
            if let Some(dd_service) = arg.strip_prefix("-Ddd.service=") {
                metadata.dd_service = Some(dd_service.to_string());
                break;
            }
        }
        
        let mut prev_arg_is_flag = false;
        
        for arg in args {
            let has_flag_prefix = arg.starts_with('-');
            let includes_assignment = arg.contains('=') ||
                arg.starts_with("-X") ||
                arg.starts_with("-javaagent:") ||
                arg.starts_with("-verbose:");
            let at_arg = arg.starts_with('@');
            let should_skip_arg = prev_arg_is_flag || has_flag_prefix || includes_assignment || at_arg;
            
            if !should_skip_arg {
                let mut arg_clean = remove_file_path(arg);
                arg_clean = trim_colon_right(&arg_clean);
                
                if is_rune_letter_at(&arg_clean, 0) {
                    // Do JEE detection to extract additional service names from context roots
                    let jee_extractor = JeeExtractor::new(ctx);
                    let (vendor, additional_names) = jee_extractor.extract_service_names();
                    
                    let mut source = ServiceNameSource::CommandLine;
                    if !additional_names.is_empty() {
                        source = Self::get_vendor_source(vendor);
                    }
                    
                    // Check for JAR or WAR files
                    if arg_clean.ends_with(JAVA_JAR_EXTENSION) || arg_clean.ends_with(JAVA_WAR_EXTENSION) {
                        // Try Spring Boot detection first if no additional names from JEE
                        if additional_names.is_empty() {
                            let mut spring_parser = SpringBootParser::new(ctx);
                            match spring_parser.get_spring_boot_app_name(arg) {
                                Ok(Some(spring_app_name)) => {
                                    let mut spring_metadata = ServiceMetadata::new(spring_app_name, ServiceNameSource::Spring);
                                    spring_metadata.dd_service = metadata.dd_service.clone();
                                    return Ok(Some(spring_metadata));
                                }
                                Ok(None) => {
                                    // Spring detection returned None, continue with fallback
                                }
                                Err(_) => {
                                    // Spring detection failed, continue with fallback detection
                                    tracing::debug!("Spring Boot detection failed for {}, falling back to JAR/WAR detection", arg);
                                }
                            }
                        }
                        
                        // Extract name from JAR/WAR file
                        let name = if arg_clean.ends_with(JAVA_JAR_EXTENSION) {
                            arg_clean.trim_end_matches(JAVA_JAR_EXTENSION).to_string()
                        } else {
                            arg_clean.trim_end_matches(JAVA_WAR_EXTENSION).to_string()
                        };
                        
                        let mut result_metadata = ServiceMetadata::new(name, source);
                        result_metadata.additional_names = additional_names;
                        result_metadata.dd_service = metadata.dd_service.clone();
                        return Ok(Some(result_metadata));
                    }
                    
                    // Check for Apache projects
                    if let Some(apache_suffix) = arg_clean.strip_prefix(JAVA_APACHE_PREFIX) {
                        if let Some(dot_idx) = apache_suffix.find('.') {
                            let project_name = apache_suffix[..dot_idx].to_string();
                            let mut result_metadata = ServiceMetadata::new(project_name, source);
                            result_metadata.additional_names = additional_names;
                            result_metadata.dd_service = metadata.dd_service.clone();
                            return Ok(Some(result_metadata));
                        }
                    }
                    
                    // Check for Spring Boot launchers
                    if arg_clean == SPRING_BOOT_LAUNCHER || arg_clean == SPRING_BOOT_OLD_LAUNCHER {
                        let mut spring_parser = SpringBootParser::new(ctx);
                        match spring_parser.get_spring_boot_launcher_app_name() {
                            Ok(Some(spring_app_name)) => {
                                let mut spring_metadata = ServiceMetadata::new(spring_app_name, ServiceNameSource::Spring);
                                spring_metadata.dd_service = metadata.dd_service.clone();
                                return Ok(Some(spring_metadata));
                            }
                            Ok(None) => {
                                // Spring launcher detection returned None, set as fallback
                                if fallback_metadata.is_none() {
                                    let mut fallback = ServiceMetadata::new(arg_clean.clone(), source);
                                    fallback.additional_names = additional_names.clone();
                                    fallback.dd_service = metadata.dd_service.clone();
                                    fallback_metadata = Some(fallback);
                                }
                            }
                            Err(_) => {
                                // Spring launcher detection failed, use launcher name as fallback
                                tracing::debug!("Spring Boot launcher detection failed, using launcher name as fallback");
                                if fallback_metadata.is_none() {
                                    let mut fallback = ServiceMetadata::new(arg_clean.clone(), source);
                                    fallback.additional_names = additional_names.clone();
                                    fallback.dd_service = metadata.dd_service.clone();
                                    fallback_metadata = Some(fallback);
                                }
                            }
                        }
                    }
                    
                    // Default case - use the argument as service name
                    let mut result_metadata = ServiceMetadata::new(arg_clean, source);
                    result_metadata.additional_names = additional_names;
                    result_metadata.dd_service = metadata.dd_service.clone();
                    return Ok(Some(result_metadata));
                }
            }
            
            prev_arg_is_flag = has_flag_prefix && !includes_assignment && !Self::is_name_flag(arg);
        }
        
        // Return fallback metadata if we have one, otherwise None
        Ok(fallback_metadata)
    }
    
    fn name(&self) -> &'static str {
        "java"
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context::Environment;
    use crate::filesystem::MemoryFileSystem;
    use std::sync::Arc;

    fn create_test_context() -> DetectionContext {
        let fs = Arc::new(MemoryFileSystem::new());
        let env = Environment::new();
        DetectionContext::new(vec![], env, fs)
    }

    #[test]
    fn test_java_detector_jar_file() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["-jar".to_string(), "myapp.jar".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_detector_war_file() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["webapp.war".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "webapp");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_detector_apache_project() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["org.apache.tomcat.BootstrapMain".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "tomcat");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_java_detector_dd_service() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["-Ddd.service=my-custom-service".to_string(), "-jar".to_string(), "app.jar".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app");
        assert_eq!(metadata.dd_service, Some("my-custom-service".to_string()));
    }

    #[test]
    fn test_java_detector_skip_flags() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "-Xmx1g".to_string(),
            "-javaagent:agent.jar".to_string(),
            "-verbose:gc".to_string(),
            "MainClass".to_string(),
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "MainClass");
    }

    #[test]
    fn test_java_detector_spring_boot_launcher() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![SPRING_BOOT_LAUNCHER.to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        // Should detect but not find Spring Boot app name since we don't have classpath setup
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, SPRING_BOOT_LAUNCHER);
    }

    #[test]
    fn test_java_detector_tomcat_detection() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        // Simulate Tomcat startup command
        let args = vec![
            "-Dcatalina.base=/opt/tomcat".to_string(),
            "org.apache.catalina.startup.Bootstrap".to_string(),
            "start".to_string(),
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        // Should extract "catalina" from the Apache project name
        assert_eq!(metadata.name, "catalina");
    }

    #[test]
    fn test_java_detector_skip_at_files() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "@/path/to/argfile".to_string(),
            "RealMainClass".to_string(),
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "RealMainClass");
    }

    #[test]
    fn test_java_detector_path_removal() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["/usr/local/bin/myapp.jar".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
    }

    #[test]
    fn test_java_detector_colon_trimming() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["com.example.Main:8080".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "com.example.Main");
    }

    #[test]
    fn test_is_name_flag() {
        assert!(JavaDetector::is_name_flag("-jar"));
        assert!(JavaDetector::is_name_flag("-m"));
        assert!(JavaDetector::is_name_flag("--module"));
        assert!(!JavaDetector::is_name_flag("-Xmx1g"));
        assert!(!JavaDetector::is_name_flag("-cp"));
    }

    #[test]
    fn test_java_detector_no_match() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["-Xmx1g".to_string(), "-cp".to_string(), "/path/to/classes".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }

    #[test]
    fn test_java_detector_empty_args() {
        let detector = JavaDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args: Vec<String> = vec![];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }
}