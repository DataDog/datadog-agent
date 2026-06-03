name "sds"

# Keep in sync with the github.com/DataDog/dd-sensitive-data-scanner/sds-go/go
# module required in pkg/util/sds/go.mod.
default_version "3c48e9fa5604"
source git: 'https://github.com/DataDog/dd-sensitive-data-scanner'

build do
    license "Apache-2.0"
    license_file "./LICENSE"

    # no Windows support for now.
    if linux_target? || osx_target?
        command "cargo build --release --features dd_sds_go", cwd: "#{project_dir}/sds"
        if osx_target?
            copy "sds/target/release/libdd_sds.dylib", "#{install_dir}/embedded/lib"
        end
        if linux_target?
            copy "sds/target/release/libdd_sds.so", "#{install_dir}/embedded/lib"
        end
    end
end
