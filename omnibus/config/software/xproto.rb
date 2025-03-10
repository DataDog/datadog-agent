
name "xproto"
default_version "7.0.27"

source url: "https://xorg.freedesktop.org/releases/individual/proto/xproto-#{version}.tar.gz",
       sha256: "693d6ae50cb642fc4de6ab1f69e3f38a8e5a67eb41ac2aca253240f999282b6b",
       extract: :seven_zip

relative_path "xproto-#{version}"

configure_env = with_standard_compiler_flags(with_embedded_path)

build do
  license "MIT"
  license_file "./COPYING"

  configure_options = [
    "--disable-static",
  ]
  configure(*configure_options, env: configure_env)
  command "make -j #{workers}", env: configure_env
  command "make -j #{workers} install", env: configure_env
end
