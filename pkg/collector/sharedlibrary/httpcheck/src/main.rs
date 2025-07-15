use std::error::Error;
use serde::Deserialize;

#[derive(Deserialize)]
struct Ip {
    origin: String,
}


fn main() -> Result<(), Box<dyn Error>> {
    let json: Ip = reqwest::blocking::get("http://httpbin.org/ip")?.json()?;
    println!("IP: {}", json.origin);
    Ok(())
}