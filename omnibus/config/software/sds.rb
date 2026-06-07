name "sds"
default_version "3c48e9fa5604"

skip_transitive_dependency_licensing true

license "Apache-2.0"
license_file "https://raw.githubusercontent.com/DataDog/dd-sensitive-data-scanner/3c48e9fa5604/LICENSE"

build do
    # No Windows support for now.
    if linux_target? || osx_target?
        command_on_repo_root "bazelisk run --config=release --//:install_dir=#{install_dir} -- //deps/sds:install --destdir=#{install_dir}"
    end
end
