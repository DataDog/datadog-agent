use std::error::Error;
use std::time::Instant;

fn main() -> Result<(), Box<dyn Error>> {
    // hardcoded variables
    let url = "https://datadoghq.com";

    let start = Instant::now();
    let response = reqwest::blocking::get(url)?;
    let duration = start.elapsed();

    if !response.status().is_success() {
        return Err(format!("Failed to fetch {}: {}", url, response.status()).into());
    }

    println!("Fetched {} in {} ms", url, duration.as_millis());
    Ok(())
}