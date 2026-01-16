use std::env;

use anyhow::{Ok, Result, bail};

mod config;
mod shlib;

mod cstring;
mod aggregator;

fn main() -> Result<()> {
    let mut args = env::args();

    let library_path = match args.nth(1) {
        Some(path) => path,
        None => bail!("Please provide a path to a shared library."),
    };

    let config_path = match args.next() {
        Some(path) => path,
        None => bail!("Please provide a path to the check configuration."),
    };

    let handle = shlib::open(&library_path)?;
    let shared_library = shlib::SharedLibrary::from_handle(&handle)?;
    
    let fake_aggregator = aggregator::fake_aggregator();
    shared_library.run(&config_path, &fake_aggregator)?;
    aggregator::print_payload_counts();
    
    println!("Library version: {}", shared_library.version()?);

    handle.close()?;

    Ok(())
}
