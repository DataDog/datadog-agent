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
    fn test_common_url() {
        let url_str = "https://example.com:1234";

        let url = Url::from(url_str);

        assert_eq!(url.scheme, Some(String::from("https")));
        assert_eq!(url.path, String::from("example.com"));
        assert_eq!(url.port, Some(String::from("1234")));
    }
}