// Performance benchmarking suite to validate Rust performance benefits over Go USM

use criterion::{black_box, criterion_group, criterion_main, Criterion, BenchmarkId};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use usm_rs::{
    context::{DetectionContext, Environment},
    detectors::java::JavaDetector,
    detectors::enhanced_detectors::{EnhancedPythonDetector, EnhancedRubyDetector, EnhancedDotNetDetector, EnhancedPhpDetector},
    frameworks::{
        spring::SpringBootParser,
        javaee::JeeExtractor,
    },
    filesystem::MemoryFileSystem,
    utils::{
        parsing::{create_property_resolver, PropertyResolver},
        zip_utils::analyze_spring_boot_jar,
    },
};

fn setup_test_filesystem() -> Arc<MemoryFileSystem> {
    let mut fs = MemoryFileSystem::new();
    
    // Add realistic Spring Boot application files
    let spring_properties = r#"
spring.application.name=${APP_NAME:spring-app}
spring.profiles.active=prod,metrics
server.port=${SERVER_PORT:8080}
management.endpoints.web.exposure.include=health,info,metrics
management.endpoint.health.show-details=when-authorized
logging.level.com.example=DEBUG
database.url=${DATABASE_URL:jdbc:postgresql://localhost:5432/mydb}
database.username=${DATABASE_USER:postgres}
database.password=${DATABASE_PASSWORD:secret}
cache.redis.url=${REDIS_URL:redis://localhost:6379}
messaging.kafka.bootstrap-servers=${KAFKA_BROKERS:localhost:9092}
"#;
    
    fs.add_file("BOOT-INF/classes/application.properties", spring_properties.as_bytes().to_vec());
    fs.add_file("BOOT-INF/classes/application-prod.properties", 
               b"spring.application.name=production-service\nserver.port=8443");
    
    // Add complex JAR structure
    for i in 0..50 {
        fs.add_file(&format!("BOOT-INF/lib/dependency-{}.jar", i), 
                   format!("mock dependency {} content", i).as_bytes().to_vec());
    }
    
    // Add Spring Boot manifest
    let manifest_content = r#"Manifest-Version: 1.0
Main-Class: org.springframework.boot.loader.launch.JarLauncher
Start-Class: com.example.Application
Spring-Boot-Version: 3.2.0
Spring-Boot-Classes: BOOT-INF/classes/
Spring-Boot-Lib: BOOT-INF/lib/
Build-Jdk-Spec: 17
Implementation-Title: My Spring App
Implementation-Version: 1.0.0
"#;
    
    fs.add_file("META-INF/MANIFEST.MF", manifest_content.as_bytes().to_vec());
    
    // Add Tomcat configuration files
    let server_xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<Server port="8005" shutdown="SHUTDOWN">
    <Service name="Catalina">
        <Engine name="Catalina" defaultHost="localhost">
            <Host name="localhost" appBase="webapps">
                <Context path="/api" docBase="api.war" />
                <Context path="/admin" docBase="admin.war" />
                <Context path="/users" docBase="users" />
                <Context path="/orders" docBase="orders.war" />
                <Context path="/inventory" docBase="inventory" />
            </Host>
        </Engine>
    </Service>
</Server>"#;
    
    fs.add_file("/opt/tomcat/conf/server.xml", server_xml.as_bytes().to_vec());
    
    // Add multiple webapp files
    for app in &["api", "admin", "users", "orders", "inventory"] {
        fs.add_file(&format!("/opt/tomcat/webapps/{}.war", app), 
                   format!("mock war file for {}", app).as_bytes().to_vec());
    }
    
    // Add Python framework files
    let django_settings = r#"
import os
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent
SECRET_KEY = os.environ.get('SECRET_KEY', 'dev-secret')
DEBUG = os.environ.get('DEBUG', 'False').lower() == 'true'
ALLOWED_HOSTS = ['localhost', '127.0.0.1']

INSTALLED_APPS = [
    'django.contrib.admin',
    'django.contrib.auth',
    'django.contrib.contenttypes',
    'django.contrib.sessions',
    'django.contrib.messages',
    'django.contrib.staticfiles',
    'rest_framework',
    'myapp',
    'users',
    'orders',
]

ROOT_URLCONF = 'myproject.urls'

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.postgresql',
        'NAME': os.environ.get('DB_NAME', 'myproject'),
        'USER': os.environ.get('DB_USER', 'postgres'),
        'PASSWORD': os.environ.get('DB_PASSWORD', ''),
        'HOST': os.environ.get('DB_HOST', 'localhost'),
        'PORT': os.environ.get('DB_PORT', '5432'),
    }
}
"#;
    
    fs.add_file("myproject/settings.py", django_settings.as_bytes().to_vec());
    
    // Add Node.js package files
    let package_json = r#"{
  "name": "@company/microservice-platform",
  "version": "2.1.0",
  "description": "Enterprise microservice platform",
  "main": "dist/server.js",
  "scripts": {
    "start": "node dist/server.js",
    "dev": "nodemon src/server.ts",
    "build": "tsc",
    "test": "jest",
    "lint": "eslint src/**/*.ts"
  },
  "dependencies": {
    "express": "^4.18.2",
    "helmet": "^7.0.0",
    "cors": "^2.8.5",
    "morgan": "^1.10.0",
    "compression": "^1.7.4",
    "dotenv": "^16.3.1",
    "@prometheus-prom/client": "^14.2.0",
    "winston": "^3.10.0"
  },
  "engines": {
    "node": ">=18.0.0",
    "npm": ">=9.0.0"
  }
}"#;
    
    fs.add_file("package.json", package_json.as_bytes().to_vec());
    
    Arc::new(fs)
}

fn bench_java_detection(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("java_detection");
    group.measurement_time(Duration::from_secs(10));
    
    // Test different Java application scenarios
    let test_cases = vec![
        ("spring_boot_jar", vec![
            "java".to_string(),
            "-Dspring.profiles.active=prod".to_string(),
            "-Dspring.application.name=benchmark-app".to_string(),
            "-jar".to_string(),
            "myapp.jar".to_string(),
        ]),
        ("tomcat_server", vec![
            "java".to_string(),
            "-Dcatalina.base=/opt/tomcat".to_string(),
            "-Dspring.config.location=classpath:/application.properties".to_string(),
            "org.apache.catalina.startup.Bootstrap".to_string(),
        ]),
        ("spring_boot_launcher", vec![
            "java".to_string(),
            "-cp".to_string(),
            "myapp.jar".to_string(),
            "org.springframework.boot.loader.launch.JarLauncher".to_string(),
        ]),
        ("complex_classpath", vec![
            "java".to_string(),
            "-cp".to_string(),
            "/opt/app/lib/*:/opt/app/config".to_string(),
            "-Dspring.application.name=complex-app".to_string(),
            "-Dspring.config.additional-location=file:/opt/app/config/".to_string(),
            "com.example.Application".to_string(),
        ]),
    ];
    
    for (name, args) in test_cases {
        group.bench_with_input(BenchmarkId::new("detect", name), &args, |b, args| {
            b.iter(|| {
                let mut env = Environment::new();
                env.set("APP_NAME".to_string(), "benchmark-service".to_string());
                env.set("SERVER_PORT".to_string(), "8080".to_string());
                
                let mut ctx = DetectionContext::new(args.clone(), env, filesystem.clone());
                let detector = JavaDetector::new(&ctx);
                
                black_box(detector.detect(&mut ctx, args).unwrap())
            })
        });
    }
    
    group.finish();
}

fn bench_spring_boot_parsing(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("spring_boot_parsing");
    group.measurement_time(Duration::from_secs(5));
    
    // Test different Spring Boot configuration scenarios
    let scenarios = vec![
        ("simple_jar", "simple.jar"),
        ("complex_jar", "myapp.jar"),
        ("fat_jar_with_deps", "enterprise-app.jar"),
    ];
    
    for (name, jar_name) in scenarios {
        group.bench_with_input(BenchmarkId::new("parse", name), &jar_name, |b, jar_name| {
            b.iter(|| {
                let mut env = Environment::new();
                env.set("APP_NAME".to_string(), "benchmark-app".to_string());
                env.set("DATABASE_URL".to_string(), "jdbc:postgresql://db:5432/prod".to_string());
                
                let args = vec![
                    "java".to_string(),
                    "-Dspring.profiles.active=prod,metrics".to_string(),
                    "-jar".to_string(),
                    jar_name.to_string(),
                ];
                
                let mut ctx = DetectionContext::new(args, env, filesystem.clone());
                let mut parser = SpringBootParser::new(&mut ctx);
                
                black_box(parser.get_spring_boot_app_name(jar_name).unwrap())
            })
        });
    }
    
    group.finish();
}

fn bench_jee_extraction(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("jee_extraction");
    group.measurement_time(Duration::from_secs(3));
    
    let jee_scenarios = vec![
        ("tomcat", vec![
            "java".to_string(),
            "-Dcatalina.base=/opt/tomcat".to_string(),
            "org.apache.catalina.startup.Bootstrap".to_string(),
        ]),
        ("jboss", vec![
            "java".to_string(),
            "-Djboss.server.base.dir=/opt/jboss/standalone".to_string(),
            "org.jboss.as.standalone".to_string(),
        ]),
        ("weblogic", vec![
            "java".to_string(),
            "-Dwls.home=/opt/oracle/weblogic".to_string(),
            "weblogic.Server".to_string(),
        ]),
    ];
    
    for (name, args) in jee_scenarios {
        group.bench_with_input(BenchmarkId::new("extract", name), &args, |b, args| {
            b.iter(|| {
                let env = Environment::new();
                let ctx = DetectionContext::new(args.clone(), env, filesystem.clone());
                let extractor = JeeExtractor::new(&ctx);
                
                black_box(extractor.extract_service_names())
            })
        });
    }
    
    group.finish();
}

fn bench_property_resolution(c: &mut Criterion) {
    let mut group = c.benchmark_group("property_resolution");
    group.measurement_time(Duration::from_secs(3));
    
    // Create complex environment and arguments
    let mut env_vars = HashMap::new();
    for i in 0..100 {
        env_vars.insert(format!("CONFIG_VAR_{}", i), format!("value_{}", i));
    }
    env_vars.insert("SPRING_APPLICATION_NAME".to_string(), "env-service".to_string());
    env_vars.insert("DATABASE_URL".to_string(), "postgres://localhost/prod".to_string());
    
    let args = vec![
        "java".to_string(),
        "-Dspring.application.name=system-service".to_string(),
        "-Dserver.port=8080".to_string(),
        "-Ddatabase.pool.size=20".to_string(),
        "--spring.application.name=cmd-service".to_string(),
        "--server.port=9090".to_string(),
        "-jar".to_string(),
        "app.jar".to_string(),
    ];
    
    group.bench_function("create_resolver", |b| {
        b.iter(|| {
            black_box(create_property_resolver(&args, &env_vars))
        })
    });
    
    group.bench_function("resolve_properties", |b| {
        let resolver = create_property_resolver(&args, &env_vars);
        b.iter(|| {
            black_box(resolver.get_property("spring.application.name"));
            black_box(resolver.get_property("server.port"));
            black_box(resolver.get_property("database.url"));
            black_box(resolver.get_property("nonexistent.property"));
        })
    });
    
    group.bench_function("resolve_placeholders", |b| {
        let resolver = create_property_resolver(&args, &env_vars);
        let complex_template = "App: ${spring.application.name} running on port ${server.port} with database ${database.url:default-db} and cache ${redis.url:redis://localhost:6379}";
        
        b.iter(|| {
            black_box(resolver.resolve_placeholders(complex_template))
        })
    });
    
    group.finish();
}

fn bench_jar_analysis(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("jar_analysis");
    group.measurement_time(Duration::from_secs(5));
    
    group.bench_function("analyze_spring_boot_jar", |b| {
        b.iter(|| {
            black_box(
                analyze_spring_boot_jar(&*filesystem, std::path::Path::new("myapp.jar")).unwrap()
            )
        })
    });
    
    group.finish();
}

fn bench_multi_detector_scenario(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("multi_detector_scenario");
    group.measurement_time(Duration::from_secs(10));
    
    // Simulate real-world scenario where multiple detectors are tried
    group.bench_function("detect_java_service", |b| {
        b.iter(|| {
            let mut env = Environment::new();
            env.set("SPRING_APPLICATION_NAME".to_string(), "production-api".to_string());
            
            let args = vec![
                "java".to_string(),
                "-Dspring.profiles.active=prod".to_string(),
                "-jar".to_string(),
                "microservice.jar".to_string(),
            ];
            
            let mut ctx = DetectionContext::new(args.clone(), env, filesystem.clone());
            
            // Try Java detector
            let java_detector = JavaDetector::new(&ctx);
            let result = java_detector.detect(&mut ctx, &args);
            
            black_box(result)
        })
    });
    
    group.bench_function("detect_python_service", |b| {
        b.iter(|| {
            let env = Environment::new();
            let args = vec![
                "python".to_string(),
                "manage.py".to_string(),
                "runserver".to_string(),
            ];
            
            let mut ctx = DetectionContext::new(args.clone(), env, filesystem.clone());
            
            // Try Python detector
            let python_detector = EnhancedPythonDetector::new(&ctx);
            let result = python_detector.detect(&mut ctx, &args);
            
            black_box(result)
        })
    });
    
    group.finish();
}

fn bench_memory_usage(c: &mut Criterion) {
    let filesystem = setup_test_filesystem();
    
    let mut group = c.benchmark_group("memory_usage");
    group.measurement_time(Duration::from_secs(5));
    
    // Test memory efficiency with many concurrent detection contexts
    group.bench_function("concurrent_contexts", |b| {
        b.iter(|| {
            let mut contexts = Vec::new();
            
            // Create 100 concurrent detection contexts
            for i in 0..100 {
                let mut env = Environment::new();
                env.set("SERVICE_ID".to_string(), format!("service-{}", i));
                
                let args = vec![
                    "java".to_string(),
                    format!("-Dspring.application.name=service-{}", i),
                    "-jar".to_string(),
                    format!("service-{}.jar", i),
                ];
                
                let ctx = DetectionContext::new(args, env, filesystem.clone());
                contexts.push(ctx);
            }
            
            black_box(contexts)
        })
    });
    
    group.finish();
}

// Criterion benchmark groups
criterion_group!(
    benches,
    bench_java_detection,
    bench_spring_boot_parsing,
    bench_jee_extraction,
    bench_property_resolution,
    bench_jar_analysis,
    bench_multi_detector_scenario,
    bench_memory_usage
);

criterion_main!(benches);

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn benchmark_smoke_test() {
        let filesystem = setup_test_filesystem();
        
        // Quick smoke test to ensure benchmarks can run
        let mut env = Environment::new();
        env.set("APP_NAME".to_string(), "test-app".to_string());
        
        let args = vec![
            "java".to_string(),
            "-jar".to_string(),
            "test.jar".to_string(),
        ];
        
        let mut ctx = DetectionContext::new(args.clone(), env, filesystem);
        let detector = JavaDetector::new(&ctx);
        
        let result = detector.detect(&mut ctx, &args);
        assert!(result.is_ok());
        
        println!("✅ Benchmark smoke test passed");
    }
}