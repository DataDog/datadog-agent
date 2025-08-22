// Gunicorn detection framework

use crate::context::Environment;

/// Extract Gunicorn application name from command line arguments
pub fn extract_gunicorn_name_from_args(args: &[String]) -> Option<String> {
    let mut name_next = false;
    let mut skip_next = false;
    let mut candidate = String::new();
    
    // List of long options that do NOT take an argument
    let no_arg_options = [
        "--reload", "--spew", "--check-config", "--print-config", "--preload",
        "--no-sendfile", "--reuse-port", "--daemon", "--initgroups", 
        "--capture-output", "--log-syslog", "--enable-stdio-inheritance",
        "--disable-redirect-access-to-syslog", "--proxy-protocol",
        "--suppress-ragged-eofs", "--do-handshake-on-connect", "--strip-header-spaces"
    ];
    
    for (_i, arg) in args.iter().enumerate() {
        if name_next {
            return Some(parse_wsgi_app_name(arg));
        }
        
        if skip_next {
            skip_next = false;
            continue;
        }
        
        if arg.starts_with("--name=") {
            return Some(arg[7..].to_string());
        } else if arg == "--name" {
            name_next = true;
        } else if arg.starts_with("--") {
            if arg.contains('=') {
                continue;
            }
            
            if !no_arg_options.contains(&arg.as_str()) {
                skip_next = true;
            }
        } else if arg.starts_with('-') {
            // Handle short options
            let chars: Vec<char> = arg[1..].chars().collect();
            for (idx, &ch) in chars.iter().enumerate() {
                match ch {
                    'n' => {
                        let rest = &chars[idx + 1..];
                        if !rest.is_empty() {
                            let name: String = rest.iter().collect();
                            return Some(name);
                        } else {
                            name_next = true;
                        }
                        break;
                    }
                    'R' | 'd' => {
                        // These options don't take arguments
                        continue;
                    }
                    _ => {
                        // Other flags take arguments
                        let rest = &chars[idx + 1..];
                        if rest.is_empty() {
                            skip_next = true;
                        }
                        break;
                    }
                }
            }
        } else if candidate.is_empty() {
            candidate = arg.clone();
        }
    }
    
    if !candidate.is_empty() {
        Some(parse_wsgi_app_name(&candidate))
    } else {
        None
    }
}

/// Extract Gunicorn application name from environment variables
pub fn extract_gunicorn_name_from_env(envs: &Environment) -> Option<String> {
    // Check GUNICORN_CMD_ARGS
    if let Some(cmd_args) = envs.get("GUNICORN_CMD_ARGS") {
        let args: Vec<String> = cmd_args.split_whitespace().map(|s| s.to_string()).collect();
        if let Some(name) = extract_gunicorn_name_from_args(&args) {
            return Some(name);
        }
    }
    
    // Check WSGI_APP
    if let Some(wsgi_app) = envs.get("WSGI_APP") {
        if !wsgi_app.is_empty() {
            return Some(parse_wsgi_app_name(wsgi_app));
        }
    }
    
    None
}

/// Parse module name from WSGI application string (e.g., "module:app" -> "module")
fn parse_wsgi_app_name(wsgi_app: &str) -> String {
    if let Some(colon_pos) = wsgi_app.find(':') {
        wsgi_app[..colon_pos].to_string()
    } else {
        wsgi_app.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_gunicorn_name_from_args() {
        let args = vec![
            "--workers".to_string(),
            "4".to_string(),
            "--name".to_string(),
            "my-app".to_string(),
            "wsgi:application".to_string(),
        ];
        
        let result = extract_gunicorn_name_from_args(&args);
        assert_eq!(result, Some("my-app".to_string()));
    }
    
    #[test]
    fn test_extract_gunicorn_name_from_wsgi() {
        let args = vec!["myapp:application".to_string()];
        
        let result = extract_gunicorn_name_from_args(&args);
        assert_eq!(result, Some("myapp".to_string()));
    }
    
    #[test]
    fn test_parse_wsgi_app_name() {
        assert_eq!(parse_wsgi_app_name("myapp:application"), "myapp");
        assert_eq!(parse_wsgi_app_name("package.module:app"), "package.module");
        assert_eq!(parse_wsgi_app_name("simpleapp"), "simpleapp");
    }
}