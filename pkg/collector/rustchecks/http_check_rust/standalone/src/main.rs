use std::env;
use std::fs::File;

use http_check::Result;
use http_check::check::{HttpCheck, config};
use http_check::sink;

// TODO: use anyhow crate?

#[tokio::main(flavor = "current_thread")]
async fn main() -> Result<()> {
    let path = env::args().nth(1).unwrap_or("./conf.yaml".to_string());
    let file = File::open(path)?;
    let config: config::Global = serde_yaml::from_reader(file)?;

    println!("{}", serde_yaml::to_string(&config).unwrap());

    let id = "standalone".to_string();
    let console = sink::Console {};
    let mut hc = HttpCheck::new(&console, id, config.init_config, config.instances[0].clone());
    hc.check().await;
    Ok(())
}
