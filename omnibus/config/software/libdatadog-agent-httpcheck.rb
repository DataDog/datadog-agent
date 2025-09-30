name "libdatadog-agent-httpcheck"
description "Shared library check `http_check`"

default_version "0.1.0"

source path: "#{project.files_path}/#{name}"

always_build true

build do
  env = with_standard_compiler_flags(with_embedded_path)
  
  # specific library extension for each platform
  if linux?
    extension = "so"
  elsif mac?
    extension = "dylib"
  elsif windows?
    extension = "dll"
  else
    extension = "so"
  end

  source_file = "#{project.files_path}/#{name}/#{name}.#{extension}"
  lib_path = "#{install_dir}/embedded/lib/#{name}.#{extension}"

  if File.exist?(source_file)
    copy source_file, "#{install_dir}/embedded/lib/"
    command "chmod 755 #{lib_path}"
  else
    raise "#{source_file} not found in #{project.files_path}/#{name}"
  end

  # verify the library was copied correctly
  block do
    if not File.exist?(lib_path)
      raise "Failed to install library: #{lib_path} not found after build"
    end
  end
end
