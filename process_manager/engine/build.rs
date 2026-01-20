fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed=proto/process_manager.proto");
    println!("cargo:rerun-if-changed=proto");

    tonic_build::configure()
        .build_client(true)
        .build_server(true)
        .compile(&["proto/process_manager.proto"], &["proto"])?;

    Ok(())
}
