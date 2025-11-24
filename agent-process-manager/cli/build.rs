fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure().compile(&["../proto/process_manager.proto"], &["../proto"])?;
    Ok(())
}
