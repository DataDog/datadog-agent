use std::env;

use anyhow::{Ok, Result, bail};

mod aggregator;

mod shlib;
use crate::{aggregator::mock_aggregator, shlib::{SharedLibrary, open}};

fn main() -> Result<()> {
    let library_path = match env::args().nth(1) {
        Some(libray_path) => libray_path,
        None => bail!("Please provide a path to a shared library."),
    };

    let handle = open(&library_path)?;
    let shared_library = SharedLibrary::from_handle(&handle)?;
    
    let aggregator = mock_aggregator();

    shared_library.run("", "", "", &aggregator)?;

    let version = shared_library.version()?;
    println!("Library version: {}", version);

    handle.close()?;

    Ok(())
}
