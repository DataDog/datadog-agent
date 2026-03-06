/// Parsed URL
pub struct Url {
    pub scheme: Option<String>,
    pub path: String,
    pub port: Option<String>,
}

impl Url {
    pub fn from(url: &str) -> Self {
        let mut remaining = url;
        
        // Extract scheme
        let scheme = if let Some(scheme_end) = remaining.find("://") {
            let extracted_scheme = remaining[..scheme_end].to_string();
            remaining = &remaining[scheme_end + 3..]; // Skip past "://"
            Some(extracted_scheme)
        } else {
            None
        };

        // Extract path and port
        let (path, port) = if let Some(port_start) = remaining.find(':') {
            let path = &remaining[..port_start];
            let port = &remaining[port_start + 1..];
            (path.to_string(), Some(port.to_string()))
        } else {
            (remaining.to_string(), None)
        };

        Self { scheme, path, port }
    }
}

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn test_url_parsing() {
        // Test full URL with scheme, hostname, and port
        let url1 = Url::from("https://localhost:5000");
        assert_eq!(url1.scheme, Some("https".to_string()));
        assert_eq!(url1.path, "localhost");
        assert_eq!(url1.port, Some("5000".to_string()));

        // Test URL without port
        let url2 = Url::from("http://example.com");
        assert_eq!(url2.scheme, Some("http".to_string()));
        assert_eq!(url2.path, "example.com");
        assert_eq!(url2.port, None);

        // Test hostname with port but no scheme
        let url3 = Url::from("localhost:8080");
        assert_eq!(url3.scheme, None);
        assert_eq!(url3.path, "localhost");
        assert_eq!(url3.port, Some("8080".to_string()));

        // Test just hostname
        let url4 = Url::from("example.com");
        assert_eq!(url4.scheme, None);
        assert_eq!(url4.path, "example.com");
        assert_eq!(url4.port, None);
    }
}
