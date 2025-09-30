name "libdatadog-agent-httpcheck"
description "Shared library check `http_check`"

default_version "0.1.0"

source path: "#{project.files_path}/#{name}"

always_build true

build do
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

  # copy the lib corresponding to the platform
  if File.exist?("#{project.files_path}/#{name}/#{name}.#{extension}")
    copy "#{project.files_path}/#{name}/#{name}.#{extension}", "#{install_dir}/embedded/lib/"
    command "chmod +x #{install_dir}/embedded/lib/#{name}.#{extension}"
  else
    raise "#{name}.#{extension} not found in #{project.files_path}/#{name}"
  end

  # verify the library was copied correctly
  block do
    if not File.exist?("#{install_dir}/embedded/lib/#{name}.#{extension}")
      raise "#{install_dir}/embedded/lib/#{name}.#{extension} not found after build"
    end
  end
end
