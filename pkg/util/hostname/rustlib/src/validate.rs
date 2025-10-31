use regex::Regex;

// Normalize host similar to Go's NormalizeHost
pub fn normalize_host(host: &str) -> Result<String, ()> {
    if host.len() > 253 {
        return Err(());
    }
    let mut out = String::with_capacity(host.len());
    for ch in host.chars() {
        match ch {
            '\u{0000}' => return Err(()),
            '\n' | '\r' | '\t' => {}
            '<' | '>' => out.push('-'),
            other => out.push(other),
        }
    }
    Ok(out)
}

pub fn valid_hostname(hostname: &str) -> Result<(), &'static str> {
    if hostname.is_empty() {
        return Err("hostname is empty");
    }
    let lower = hostname.to_ascii_lowercase();
    match lower.as_str() {
        "localhost" | "localhost.localdomain" | "localhost6.localdomain6" | "ip6-localhost" => {
            return Err("local hostname");
        }
        _ => {}
    }
    if hostname.len() > 255 {
        return Err("name exceeded the maximum length of 255 characters");
    }
    let re = Regex::new(r"^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$").unwrap();
    if !re.is_match(hostname) {
        return Err("not RFC1123 compliant");
    }
    Ok(())
}

pub fn clean_hostname_dir(hostname: &str) -> String {
    // Replace all non [a-zA-Z0-9_-] with '_', then truncate to 32 chars
    let re = Regex::new(r"[^a-zA-Z0-9_-]+").unwrap();
    let cleaned = re.replace_all(hostname, "_").to_string();
    const MAX: usize = 32;
    if cleaned.len() > MAX { cleaned[..MAX].to_string() } else { cleaned }
}
