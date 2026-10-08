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
    Dir.glob("*.tar.xz", base: input_dir).each do |input|
      path = File.join(input_dir, input)
      shellout! "tar xf #{path} -C /"
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
    # The README depends on the type of package being built, so it must be
    # generated during packaging, not building the tarballs. When we eliminate
    # the tarball intermediary, we can fold this into the .deb/.rpm target.
    host_distribution = ""
    if not Omnibus::Config.host_distribution().nil?
      host_distribution = "--//packages/agent:host_distribution=#{Omnibus::Config.host_distribution()}"
    end
    command "bazel run #{omnibazel_flags} #{host_distribution} -- //packages/installer:install_readme --destdir=#{install_dir}"
  end
end
