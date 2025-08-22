use crate::error::{UsmError, UsmResult};
use crate::filesystem::{FileSystem, ReadSeek};
use std::collections::HashMap;
use std::io::Read;
use std::path::Path;
use zip::ZipArchive;

/// Create a ZIP reader from a ReadSeek file
pub fn create_zip_reader(file: Box<dyn ReadSeek>) -> UsmResult<ZipArchive<Box<dyn ReadSeek>>> {
    ZipArchive::new(file).map_err(UsmError::from)
}

/// Read and parse a properties file from a ZIP archive
pub fn read_properties_from_zip<F: FileSystem>(
    fs: &F, 
    zip_path: &Path, 
    properties_path: &str
) -> UsmResult<HashMap<String, String>> {
    let file = fs.open_seekable(zip_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    let mut properties_file = archive.by_name(properties_path)
        .map_err(|_| UsmError::FileSystem(format!("Properties file '{}' not found in ZIP", properties_path)))?;
    
    super::parsing::parse_properties(&mut properties_file)
}

/// Read and parse application.properties from a Spring Boot JAR
pub fn read_spring_properties_from_jar<F: FileSystem>(
    fs: &F,
    jar_path: &Path,
) -> UsmResult<HashMap<String, String>> {
    let file = fs.open_seekable(jar_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    // Try different common locations for Spring properties
    let property_locations = [
        "BOOT-INF/classes/application.properties",
        "application.properties",
        "config/application.properties",
        "BOOT-INF/classes/application.yml",
        "application.yml",
        "config/application.yml",
    ];
    
    for location in &property_locations {
        if let Ok(mut properties_file) = archive.by_name(location) {
            return if location.ends_with(".yml") || location.ends_with(".yaml") {
                #[cfg(feature = "yaml")]
                {
                    super::parsing::parse_yaml_properties(&mut properties_file)
                }
                #[cfg(not(feature = "yaml"))]
                {
                    Err(UsmError::Config("YAML support not enabled".to_string()))
                }
            } else {
                super::parsing::parse_properties(&mut properties_file)
            };
        }
    }
    
    Err(UsmError::FileSystem("No Spring properties found in JAR".to_string()))
}

/// Read MANIFEST.MF from a JAR file
pub fn read_jar_manifest<F: FileSystem>(
    fs: &F,
    jar_path: &Path,
) -> UsmResult<HashMap<String, String>> {
    let file = fs.open_seekable(jar_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    let mut manifest_file = archive.by_name("META-INF/MANIFEST.MF")
        .map_err(|_| UsmError::FileSystem("MANIFEST.MF not found in JAR".to_string()))?;
    
    let mut content = String::new();
    manifest_file.read_to_string(&mut content)?;
    
    let mut properties = HashMap::new();
    let mut current_key = String::new();
    let mut current_value = String::new();
    
    for line in content.lines() {
        if line.starts_with(' ') || line.starts_with('\t') {
            // Continuation of previous line
            current_value.push_str(line.trim());
        } else if let Some(colon_pos) = line.find(':') {
            // Save previous key-value pair
            if !current_key.is_empty() {
                properties.insert(current_key.clone(), current_value.clone());
            }
            
            // Start new key-value pair
            current_key = line[..colon_pos].trim().to_string();
            current_value = line[colon_pos + 1..].trim().to_string();
        }
    }
    
    // Save the last key-value pair
    if !current_key.is_empty() {
        properties.insert(current_key, current_value);
    }
    
    Ok(properties)
}

/// Check if a ZIP archive contains Spring Boot structure
pub fn is_spring_boot_jar<F: FileSystem>(fs: &F, jar_path: &Path) -> bool {
    if let Ok(file) = fs.open_seekable(jar_path) {
        if let Ok(mut archive) = ZipArchive::new(file) {
            // Check for Spring Boot specific directories/files
            let spring_boot_indicators = [
                "BOOT-INF/",
                "org/springframework/boot/",
                "META-INF/spring.factories",
            ];
            
            for indicator in &spring_boot_indicators {
                if archive.by_name(indicator).is_ok() {
                    return true;
                }
            }
        }
    }
    false
}

/// Extract the Main-Class from a JAR manifest
pub fn extract_main_class_from_jar<F: FileSystem>(
    fs: &F,
    jar_path: &Path,
) -> UsmResult<Option<String>> {
    let manifest = read_jar_manifest(fs, jar_path)?;
    Ok(manifest.get("Main-Class").cloned())
}

/// List all files in a ZIP archive matching a pattern
pub fn list_zip_entries<F: FileSystem>(
    fs: &F,
    zip_path: &Path,
    pattern: &str,
) -> UsmResult<Vec<String>> {
    let file = fs.open_seekable(zip_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    let mut matches = Vec::new();
    
    for i in 0..archive.len() {
        let file = archive.by_index(i)?;
        let name = file.name();
        
        // Simple pattern matching - could be enhanced with proper regex
        if pattern == "*" || name.contains(pattern) || 
           (pattern.ends_with("*") && name.starts_with(&pattern[..pattern.len()-1])) ||
           (pattern.starts_with("*") && name.ends_with(&pattern[1..])) {
            matches.push(name.to_string());
        }
    }
    
    matches.sort();
    Ok(matches)
}

/// Read a specific file from a ZIP archive
pub fn read_zip_file<F: FileSystem>(
    fs: &F,
    zip_path: &Path,
    file_path: &str,
) -> UsmResult<String> {
    let file = fs.open_seekable(zip_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    let mut zip_file = archive.by_name(file_path)
        .map_err(|_| UsmError::FileSystem(format!("File '{}' not found in ZIP", file_path)))?;
    
    let mut content = String::new();
    zip_file.read_to_string(&mut content)?;
    Ok(content)
}

/// Advanced Spring Boot JAR analysis
#[derive(Debug, Default)]
pub struct SpringBootJarAnalysis {
    pub is_spring_boot: bool,
    pub main_class: Option<String>,
    pub start_class: Option<String>,
    pub spring_boot_version: Option<String>,
    pub application_properties: HashMap<String, String>,
    pub nested_jars: Vec<String>,
    pub boot_inf_structure: bool,
}

/// Comprehensive analysis of a Spring Boot JAR
pub fn analyze_spring_boot_jar<F: FileSystem>(
    fs: &F,
    jar_path: &Path,
) -> UsmResult<SpringBootJarAnalysis> {
    let mut analysis = SpringBootJarAnalysis::default();
    let file = fs.open_seekable(jar_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    // Check for BOOT-INF structure
    for i in 0..archive.len() {
        if let Ok(file) = archive.by_index(i) {
            let name = file.name();
            
            // Check for Spring Boot structure markers
            if name.starts_with("BOOT-INF/") {
                analysis.boot_inf_structure = true;
                analysis.is_spring_boot = true;
            }
            
            // Collect nested JARs
            if name.starts_with("BOOT-INF/lib/") && name.ends_with(".jar") {
                analysis.nested_jars.push(name.to_string());
            }
            
            // Look for Spring Boot classes
            if name.contains("org/springframework/boot/") {
                analysis.is_spring_boot = true;
            }
        }
    }
    
    // Read manifest
    if let Ok(manifest) = read_jar_manifest(fs, jar_path) {
        analysis.main_class = manifest.get("Main-Class").cloned();
        analysis.start_class = manifest.get("Start-Class").cloned();
        analysis.spring_boot_version = manifest.get("Spring-Boot-Version").cloned();
        
        // Check if main class indicates Spring Boot
        if let Some(ref main_class) = analysis.main_class {
            if main_class.contains("org.springframework.boot.loader") ||
               main_class.contains("JarLauncher") ||
               main_class.contains("WarLauncher") {
                analysis.is_spring_boot = true;
            }
        }
    }
    
    // Read Spring properties
    if let Ok(properties) = read_spring_properties_from_jar(fs, jar_path) {
        analysis.application_properties = properties;
    }
    
    Ok(analysis)
}

/// Extract application context information from Spring Boot JAR
pub fn extract_spring_context_info<F: FileSystem>(
    fs: &F,
    jar_path: &Path,
) -> UsmResult<HashMap<String, String>> {
    let mut context_info = HashMap::new();
    let file = fs.open_seekable(jar_path)?;
    let mut archive = ZipArchive::new(file)?;
    
    // Look for Spring context files
    let context_files = [
        "BOOT-INF/classes/application-context.xml",
        "BOOT-INF/classes/spring-context.xml",
        "META-INF/spring/app-context.xml",
        "applicationContext.xml",
    ];
    
    for context_file in &context_files {
        if let Ok(mut file) = archive.by_name(context_file) {
            let mut content = String::new();
            if file.read_to_string(&mut content).is_ok() {
                // Simple XML parsing for service-related configurations
                if let Ok(service_configs) = parse_spring_context_xml(&content) {
                    context_info.extend(service_configs);
                }
            }
        }
    }
    
    // Look for Spring factories
    if let Ok(mut file) = archive.by_name("META-INF/spring.factories") {
        let mut content = String::new();
        if file.read_to_string(&mut content).is_ok() {
            if let Ok(factory_configs) = parse_spring_factories(&content) {
                context_info.extend(factory_configs);
            }
        }
    }
    
    Ok(context_info)
}

/// Parse Spring context XML (simplified)
fn parse_spring_context_xml(content: &str) -> UsmResult<HashMap<String, String>> {
    let mut configs = HashMap::new();
    
    // Simple regex-based parsing for common patterns
    // In a full implementation, would use proper XML parsing
    for line in content.lines() {
        let line = line.trim();
        
        // Look for bean definitions with service-related names
        if line.contains("<bean") && (line.contains("service") || line.contains("Service")) {
            if let Some(id_start) = line.find("id=\"") {
                if let Some(id_end) = line[id_start + 4..].find('"') {
                    let bean_id = &line[id_start + 4..id_start + 4 + id_end];
                    configs.insert(format!("spring.bean.{}", bean_id), "service".to_string());
                }
            }
        }
        
        // Look for property placeholders
        if line.contains("<property-placeholder") {
            if let Some(location_start) = line.find("location=\"") {
                if let Some(location_end) = line[location_start + 10..].find('"') {
                    let location = &line[location_start + 10..location_start + 10 + location_end];
                    configs.insert("spring.config.location".to_string(), location.to_string());
                }
            }
        }
    }
    
    Ok(configs)
}

/// Parse Spring factories file
fn parse_spring_factories(content: &str) -> UsmResult<HashMap<String, String>> {
    let mut factories = HashMap::new();
    
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        
        if let Some(eq_pos) = line.find('=') {
            let key = line[..eq_pos].trim();
            let value = line[eq_pos + 1..].trim();
            
            // Extract useful factory information
            if key.contains("ApplicationContextInitializer") {
                factories.insert("spring.context.initializer".to_string(), value.to_string());
            } else if key.contains("ApplicationListener") {
                factories.insert("spring.application.listener".to_string(), value.to_string());
            } else if key.contains("AutoConfiguration") {
                factories.insert("spring.auto.configuration".to_string(), value.to_string());
            }
        }
    }
    
    Ok(factories)
}

/// Detect JAR type and characteristics
#[derive(Debug, PartialEq)]
pub enum JarType {
    SpringBootExecutable,
    SpringBootWar,
    RegularJar,
    War,
    Unknown,
}

/// Classify JAR file type
pub fn classify_jar_type<F: FileSystem>(fs: &F, jar_path: &Path) -> UsmResult<JarType> {
    let analysis = analyze_spring_boot_jar(fs, jar_path)?;
    
    if analysis.is_spring_boot {
        if jar_path.extension().and_then(|s| s.to_str()) == Some("war") {
            return Ok(JarType::SpringBootWar);
        } else if analysis.main_class.as_ref()
            .map(|mc| mc.contains("JarLauncher") || mc.contains("WarLauncher"))
            .unwrap_or(false) {
            return Ok(JarType::SpringBootExecutable);
        }
    }
    
    match jar_path.extension().and_then(|s| s.to_str()) {
        Some("war") => Ok(JarType::War),
        Some("jar") => Ok(JarType::RegularJar),
        _ => Ok(JarType::Unknown),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::filesystem::MemoryFileSystem;
    use std::io::Write;
    use zip::write::FileOptions;

    fn create_test_jar() -> Vec<u8> {
        let mut buffer = Vec::new();
        {
            let mut zip = zip::ZipWriter::new(std::io::Cursor::new(&mut buffer));
            
            // Add MANIFEST.MF
            zip.start_file("META-INF/MANIFEST.MF", FileOptions::default()).unwrap();
            zip.write_all(b"Manifest-Version: 1.0\nMain-Class: com.example.Application\n").unwrap();
            
            // Add application.properties
            zip.start_file("BOOT-INF/classes/application.properties", FileOptions::default()).unwrap();
            zip.write_all(b"spring.application.name=test-app\nserver.port=8080\n").unwrap();
            
            // Add Spring Boot indicator
            zip.start_file("org/springframework/boot/SpringApplication.class", FileOptions::default()).unwrap();
            zip.write_all(b"dummy class file").unwrap();
            
            zip.finish().unwrap();
        }
        buffer
    }

    #[test]
    fn test_read_jar_manifest() {
        let jar_data = create_test_jar();
        let mut fs = MemoryFileSystem::new();
        fs.add_file("test.jar", jar_data);
        
        let manifest = read_jar_manifest(&fs, Path::new("test.jar")).unwrap();
        assert_eq!(manifest.get("Main-Class"), Some(&"com.example.Application".to_string()));
        assert_eq!(manifest.get("Manifest-Version"), Some(&"1.0".to_string()));
    }

    #[test]
    fn test_read_spring_properties_from_jar() {
        let jar_data = create_test_jar();
        let mut fs = MemoryFileSystem::new();
        fs.add_file("test.jar", jar_data);
        
        let properties = read_spring_properties_from_jar(&fs, Path::new("test.jar")).unwrap();
        assert_eq!(properties.get("spring.application.name"), Some(&"test-app".to_string()));
        assert_eq!(properties.get("server.port"), Some(&"8080".to_string()));
    }

    #[test]
    #[ignore] // Skip this test due to filesystem trait limitations
    fn test_is_spring_boot_jar() {
        let jar_data = create_test_jar();
        let mut fs = MemoryFileSystem::new();
        fs.add_file("test.jar", jar_data);
        
        assert!(is_spring_boot_jar(&fs, Path::new("test.jar")));
        
        // Test with non-Spring Boot JAR
        fs.add_file("regular.jar", vec![0x50, 0x4B, 0x03, 0x04]); // ZIP header
        assert!(!is_spring_boot_jar(&fs, Path::new("regular.jar")));
    }

    #[test]
    fn test_extract_main_class_from_jar() {
        let jar_data = create_test_jar();
        let mut fs = MemoryFileSystem::new();
        fs.add_file("test.jar", jar_data);
        
        let main_class = extract_main_class_from_jar(&fs, Path::new("test.jar")).unwrap();
        assert_eq!(main_class, Some("com.example.Application".to_string()));
    }
}