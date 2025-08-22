// Enhanced detector implementations for better compatibility

use crate::context::{DetectionContext, FrameworkHint, FrameworkType};
use crate::detectors::Detector;
use crate::error::{UsmResult, ResultExt};
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use std::collections::HashMap;

/// Enhanced Python detector with Django/Flask/FastAPI support
pub struct EnhancedPythonDetector;

impl EnhancedPythonDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(Self)
    }
    
    /// Extract Django service name from settings.py
    fn extract_django_service_name(&self, ctx: &mut DetectionContext, _args: &[String]) -> UsmResult<Option<String>> {
        // Look for Django settings files
        let settings_patterns = [
            "settings.py",
            "*/settings.py", 
            "config/settings.py",
            "project/settings.py"
        ];
        
        for pattern in &settings_patterns {
            if let Ok(files) = ctx.filesystem.glob(&std::path::PathBuf::from("."), pattern) {
                for file in files {
                    if let Ok(mut reader) = ctx.filesystem.open(&file) {
                        let mut content = String::new();
                        if reader.read_to_string(&mut content).is_ok() {
                            if let Some(name) = extract_django_app_name(&content) {
                                return Ok(Some(name));
                            }
                        }
                    }
                }
            }
        }
        
        Ok(None)
    }
    
    /// Extract Flask service name
    fn extract_flask_service_name(&self, ctx: &mut DetectionContext, _args: &[String]) -> UsmResult<Option<String>> {
        // Look for Flask app.py or main.py files
        let flask_patterns = ["app.py", "main.py", "run.py", "*/app.py"];
        
        for pattern in &flask_patterns {
            if let Ok(files) = ctx.filesystem.glob(&std::path::PathBuf::from("."), pattern) {
                for file in files {
                    if let Ok(mut reader) = ctx.filesystem.open(&file) {
                        let mut content = String::new();
                        if reader.read_to_string(&mut content).is_ok() {
                            if content.contains("Flask(__name__)") || content.contains("from flask import") {
                                if let Some(name) = file.file_stem().and_then(|s| s.to_str()) {
                                    return Ok(Some(name.to_string()));
                                }
                            }
                        }
                    }
                }
            }
        }
        
        Ok(None)
    }
    
    /// Extract FastAPI service name
    fn extract_fastapi_service_name(&self, ctx: &mut DetectionContext, _args: &[String]) -> UsmResult<Option<String>> {
        let fastapi_patterns = ["main.py", "app.py", "api.py", "*/main.py"];
        
        for pattern in &fastapi_patterns {
            if let Ok(files) = ctx.filesystem.glob(&std::path::PathBuf::from("."), pattern) {
                for file in files {
                    if let Ok(mut reader) = ctx.filesystem.open(&file) {
                        let mut content = String::new();
                        if reader.read_to_string(&mut content).is_ok() {
                            if content.contains("FastAPI()") || content.contains("from fastapi import") {
                                if let Some(name) = file.file_stem().and_then(|s| s.to_str()) {
                                    return Ok(Some(name.to_string()));
                                }
                            }
                        }
                    }
                }
            }
        }
        
        Ok(None)
    }
}

impl Detector for EnhancedPythonDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Django detection
        if let Some(django_name) = self.extract_django_service_name(ctx, args).log_and_continue() {
            ctx.set_framework_hint(FrameworkHint {
                framework_type: FrameworkType::Django,
                confidence: 0.9,
                evidence: vec!["django_settings_found".to_string()],
                suggested_service_name: Some(django_name.clone()),
            });
            return Ok(Some(ServiceMetadata::new(django_name, ServiceNameSource::Django)));
        }
        
        // Flask detection  
        if let Some(flask_name) = self.extract_flask_service_name(ctx, args).log_and_continue() {
            ctx.set_framework_hint(FrameworkHint {
                framework_type: FrameworkType::Flask,
                confidence: 0.8,
                evidence: vec!["flask_app_found".to_string()],
                suggested_service_name: Some(flask_name.clone()),
            });
            return Ok(Some(ServiceMetadata::new(flask_name, ServiceNameSource::Flask)));
        }
        
        // FastAPI detection
        if let Some(fastapi_name) = self.extract_fastapi_service_name(ctx, args).log_and_continue() {
            ctx.set_framework_hint(FrameworkHint {
                framework_type: FrameworkType::FastAPI,
                confidence: 0.8,
                evidence: vec!["fastapi_app_found".to_string()],
                suggested_service_name: Some(fastapi_name.clone()),
            });
            return Ok(Some(ServiceMetadata::new(fastapi_name, ServiceNameSource::FastAPI)));
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "enhanced_python"
    }
}

/// Enhanced Ruby detector with Rails/Sinatra support
pub struct EnhancedRubyDetector;

impl EnhancedRubyDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(Self)
    }
    
    fn extract_rails_service_name(&self, ctx: &mut DetectionContext) -> UsmResult<Option<String>> {
        // Look for Rails application.rb
        let app_config_path = std::path::PathBuf::from("config/application.rb");
        if ctx.filesystem.exists(&app_config_path) {
            if let Ok(mut reader) = ctx.filesystem.open(&app_config_path) {
                let mut content = String::new();
                if reader.read_to_string(&mut content).is_ok() {
                    // Extract module name from Rails::Application
                    for line in content.lines() {
                        if line.contains("< Rails::Application") {
                            if let Some(module_name) = line.split("::").next() {
                                let clean_name = module_name.trim().to_lowercase();
                                return Ok(Some(clean_name));
                            }
                        }
                    }
                }
            }
        }
        
        // Fallback: check directory name
        if let Ok(current_dir) = std::env::current_dir() {
            if let Some(dir_name) = current_dir.file_name().and_then(|s| s.to_str()) {
                return Ok(Some(dir_name.to_string()));
            }
        }
        
        Ok(None)
    }
}

impl Detector for EnhancedRubyDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Check for Rails
        if ctx.filesystem.exists(&std::path::PathBuf::from("config/application.rb")) ||
           ctx.filesystem.exists(&std::path::PathBuf::from("Gemfile")) {
            if let Some(rails_name) = self.extract_rails_service_name(ctx).log_and_continue() {
                ctx.set_framework_hint(FrameworkHint {
                    framework_type: FrameworkType::Rails,
                    confidence: 0.9,
                    evidence: vec!["rails_application_rb_found".to_string()],
                    suggested_service_name: Some(rails_name.clone()),
                });
                return Ok(Some(ServiceMetadata::new(rails_name, ServiceNameSource::Rails)));
            }
        }
        
        // Basic Ruby script detection
        for arg in args {
            if arg.ends_with(".rb") {
                if let Some(name) = std::path::Path::new(arg).file_stem().and_then(|s| s.to_str()) {
                    return Ok(Some(ServiceMetadata::new(name.to_string(), ServiceNameSource::CommandLine)));
                }
            }
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "enhanced_ruby"
    }
}

/// Enhanced .NET detector
pub struct EnhancedDotNetDetector;

impl EnhancedDotNetDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(Self)
    }
}

impl Detector for EnhancedDotNetDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        for arg in args {
            if arg.ends_with(".dll") {
                if let Some(name) = std::path::Path::new(arg).file_stem().and_then(|s| s.to_str()) {
                    // Check if it's ASP.NET Core by looking for configuration files
                    if ctx.filesystem.exists(&std::path::PathBuf::from("appsettings.json")) ||
                       ctx.filesystem.exists(&std::path::PathBuf::from("Program.cs")) {
                        ctx.set_framework_hint(FrameworkHint {
                            framework_type: FrameworkType::DotNetCore,
                            confidence: 0.8,
                            evidence: vec!["aspnet_config_found".to_string()],
                            suggested_service_name: Some(name.to_string()),
                        });
                        return Ok(Some(ServiceMetadata::new(name.to_string(), ServiceNameSource::AspNet)));
                    }
                    
                    return Ok(Some(ServiceMetadata::new(name.to_string(), ServiceNameSource::CommandLine)));
                }
            }
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "enhanced_dotnet"
    }
}

/// Enhanced PHP detector
pub struct EnhancedPhpDetector;

impl EnhancedPhpDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(Self)
    }
}

impl Detector for EnhancedPhpDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Check for Laravel
        if ctx.filesystem.exists(&std::path::PathBuf::from("artisan")) ||
           ctx.filesystem.exists(&std::path::PathBuf::from("composer.json")) {
            if let Ok(mut reader) = ctx.filesystem.open(&std::path::PathBuf::from("composer.json")) {
                let mut content = String::new();
                if reader.read_to_string(&mut content).is_ok() && content.contains("laravel") {
                    // Extract project name from composer.json
                    if let Ok(json) = serde_json::from_str::<serde_json::Value>(&content) {
                        if let Some(name) = json.get("name").and_then(|n| n.as_str()) {
                            let project_name = name.split('/').last().unwrap_or(name);
                            ctx.set_framework_hint(FrameworkHint {
                                framework_type: FrameworkType::Laravel,
                                confidence: 0.9,
                                evidence: vec!["laravel_composer_found".to_string()],
                                suggested_service_name: Some(project_name.to_string()),
                            });
                            return Ok(Some(ServiceMetadata::new(project_name.to_string(), ServiceNameSource::Laravel)));
                        }
                    }
                }
            }
        }
        
        // Basic PHP script detection
        for arg in args {
            if arg.ends_with(".php") {
                if let Some(name) = std::path::Path::new(arg).file_stem().and_then(|s| s.to_str()) {
                    return Ok(Some(ServiceMetadata::new(name.to_string(), ServiceNameSource::CommandLine)));
                }
            }
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "enhanced_php"
    }
}

// Helper functions

fn extract_django_app_name(content: &str) -> Option<String> {
    for line in content.lines() {
        if line.contains("ROOT_URLCONF") {
            if let Some(start) = line.find('\'').or_else(|| line.find('"')) {
                if let Some(end) = line[start+1..].find('\'').or_else(|| line[start+1..].find('"')) {
                    let app_name = &line[start+1..start+1+end];
                    if let Some(dot_pos) = app_name.find('.') {
                        return Some(app_name[..dot_pos].to_string());
                    }
                }
            }
        }
    }
    None
}

use std::io::Read;