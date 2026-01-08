fn main() -> Result<(), Box<dyn std::error::Error>> {
    let out_dir = std::path::PathBuf::from(std::env::var("OUT_DIR").unwrap());
    tonic_build::configure()
        .file_descriptor_set_path(out_dir.join("proto_descriptor.bin"))
        .protoc_arg("--experimental_allow_proto3_optional")
        .compile(&["../proto/process_manager.proto"], &["../proto"])?;
    Ok(())
}
