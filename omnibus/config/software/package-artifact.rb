name 'package-artifact'

description "Helper to package an XZ build artifact to deb/rpm/..."

always_build true

build do
  input_dir = ENV['OMNIBUS_PACKAGE_ARTIFACT_DIR']
  # Iterate over all provided intermediate artifacts. There can be the main one
  # which contains all binaries, and an optional debug one with the debuging symbols
  # that have been stripped out during the build
  # We want this in a `block` to have access to the Builder DSL
  block "Extract intermediate build artifacts" do
    # `tar xf` auto-detects xz and pipes through a single-threaded `xz -d`, so
    # the large (~GB uncompressed) artifact decompresses on one core even though
    # the deb runner has many. The upstream artifact is produced with
    # compression_threads>1 (multi-block xz), so it is parallel-decompressible.
    # Use `xz -T0` to spread decompression across all cores; fall back to the
    # original single-threaded extraction if xz lacks multithreaded decode.
    parallel_xz = begin
      shellout!("xz --help 2>&1 | grep -q -- '--threads' && echo yes || echo no").stdout.strip == "yes"
    rescue StandardError
      false
    end
    Dir.glob("*.tar.xz", base: input_dir).each do |input|
      path = File.join(input_dir, input)
      if parallel_xz
        shellout! "xz -dc -T0 #{path} | tar x -C /"
      else
        shellout! "tar xf #{path} -C /"
      end
      FileUtils.rm path
    end
  end

  unless project.name == "ddot"
    # Merge version manifests together
    # The agent file is the main one, with no .$product suffix.
    # We will merge suffixed files into the main one
    block "Merge version-manifest.json" do
      main_json_manifest = "#{install_dir}/version-manifest.json"
      versions = FFI_Yajl::Parser.parse(File.read(main_json_manifest))
      Dir.glob("#{install_dir}/version-manifest.*.json").each do |version_manifest_json_path|
        additional_versions = FFI_Yajl::Parser.parse(File.read(version_manifest_json_path))

        versions["software"].merge!(additional_versions["software"])
        FileUtils.rm version_manifest_json_path
      end
      File.open(main_json_manifest, "w") do |f|
        f.write(FFI_Yajl::Encoder.encode(versions.to_hash, pretty: true))
      end
    end

    block "Merge version-manifest.txt" do
      main_txt_manifest = "#{install_dir}/version-manifest.txt"
      Dir.glob("#{install_dir}/version-manifest.*.txt").each do |version_manifest_txt_path|
        # Simply append the listing part. The first 4 lines are the package name, blank lines
        # listing headers and a separator.
        shellout! "tail -n +5 #{version_manifest_txt_path} >> #{main_txt_manifest}"
        FileUtils.rm version_manifest_txt_path
      end
    end
  end

  if project.name == "installer"
    # This file depends on the type of package and must therefor be generated during
    # packaging, not building.
    uninstall_command="sudo yum remove datadog-installer"
    if debian_target?
        uninstall_command="sudo apt-get remove datadog-installer"
    end
    # Omnibus hardcodes the template rendering to be in config/templates/<software-name>
    # so we need to move the input to its expected location
    FileUtils.mkdir_p "#{Omnibus::Config.project_root()}/config/templates/package-artifact"
    FileUtils.move "#{Omnibus::Config.project_root()}/config/templates/installer/README.md.erb", "#{Omnibus::Config.project_root()}/config/templates/package-artifact/README.md.erb"
    erb source: "README.md.erb",
       dest: "#{install_dir}/README.md",
       mode: 0644,
       vars: { uninstall_command: uninstall_command}
  end
end
