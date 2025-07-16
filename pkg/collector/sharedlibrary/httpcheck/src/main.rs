use std::time::Instant;

fn main() {
    fn fetch(url: &str, timeout: u64) -> (Result<reqwest::blocking::Response, reqwest::Error>, std::time::Duration) {
        let client = reqwest::blocking::Client::new()
            .get(url)
            .timeout(std::time::Duration::from_millis(timeout));

        let start = Instant::now();
        let res = client.send();
        let duration = start.elapsed();

        (res, duration)
    }

    // hardcoded variables
    let url = "https://datadoghq.com";

    let (response, duration) = fetch(url, 10000);

    match response {
        Ok(resp) => {
            if resp.status().is_success() {
                println!("Successfully fetched {} in {} ms", url, duration.as_millis());
            } else {
                println!("Failed to fetch {}: HTTP {}", url, resp.status());
            }
        }
        Err(e) => {
            if e.is_timeout() {
                eprintln!("Timeout while fetching {} after {} ms", url, duration.as_millis());
            } else {
                eprintln!("Error fetching {}: {}", url, e);
            }
        }
    }
}