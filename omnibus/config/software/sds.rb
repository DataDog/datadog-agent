name "sds"

default_version "v0.1.0"
source git: 'https://github.com/DataDog/dd-sensitive-data-scanner'

build do
    license "Apache-2.0"
    license_file "./LICENSE"

    # no Windows support for now.
    if linux_target? || osx_target?
        command "cargo build --release", cwd: "#{project_dir}/sds-go/rust"
        if osx_target?
            copy "sds-go/rust/target/release/libsds_go.dylib", "#{install_dir}/embedded/lib"
        end
        if linux_target?
            copy "sds-go/rust/target/release/libsds_go.so", "#{install_dir}/embedded/lib"
        end
    end
end
