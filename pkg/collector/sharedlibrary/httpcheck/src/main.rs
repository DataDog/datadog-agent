use std::error::Error;
use serde::Deserialize;

#[derive(Deserialize)]
struct Ip {
    origin: String,
}

fn main() -> Result<(), Box<dyn Error>> {
    let url = "http://httpbin.org/ip";

    let json: Ip = reqwest::blocking::get(url)?.json()?;
    println!("IP: {}", json.origin);
    Ok(())
}