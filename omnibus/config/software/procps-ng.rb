name "procps-ng"
default_version "3.3.16"

ship_source true

source url:    "https://gitlab.com/procps-ng/procps/-/archive/v3.3.16/procps-v#{version}.tar.gz",
       sha256: "7f09945e73beac5b12e163a7ee4cae98bcdd9a505163b6a060756f462907ebbc"

relative_path "procps-v#{version}"

build do
  license "GPL-2.0"
  license_file "https://gitlab.com/procps-ng/procps/raw/master/COPYING"
  license_file "https://gitlab.com/procps-ng/procps/raw/master/COPYING.LIB"

  # By default procps-ng will build with the 'UNKNOWN' version if not built
  # from a git repository and the '.tarball-version' file doesn't exist.
  # Setting the version in that file will allow binaries to return the correct
  # info from the '--version' command.
  File.open(".tarball-version", "w") do |f|
    f.puts "#{version}"
  end

  env = with_standard_compiler_flags(with_embedded_path)
  command("./autogen.sh", env: env)
  configure_options = [
    "--without-ncurses",
    "--disable-nls",
  ]
  configure(*configure_options, env: env)
  command "make -j #{workers}", env: { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
  command "make install"

  delete "#{install_dir}/embedded/lib/libprocps.a"
end
