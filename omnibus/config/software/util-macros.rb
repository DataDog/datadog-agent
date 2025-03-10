
name "util-macros"
default_version "1.19.0"

source url: "https://xorg.freedesktop.org/releases/individual/util/util-macros-#{version}.tar.gz",
       sha256: "0d4df51b29023daf2f63aebf3ebc638ea88efedfd560ab5866741ab3f92acaa1",
       extract: :seven_zip

relative_path "util-macros-#{version}"

configure_env =
  case ohai["platform"]
  when "aix"
    {
      "CC" => "xlc -q64",
      "CXX" => "xlC -q64",
      "LD" => "ld -b64",
      "CFLAGS" => "-q64 -I#{install_dir}/embedded/include -O",
      "LDFLAGS" => "-q64 -Wl,-blibpath:/usr/lib:/lib",
      "OBJECT_MODE" => "64",
      "ARFLAGS" => "-X64 cru",
    }
  when "mac_os_x"
    {
      "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
      "CFLAGS" => "-I#{install_dir}/embedded/include -L#{install_dir}/embedded/lib",
    }
  when "solaris2"
    {
      "LDFLAGS" => "-R#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include -static-libgcc",
      "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    }
  else
    {
      "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
      "CFLAGS" => "-I#{install_dir}/embedded/include -L#{install_dir}/embedded/lib",
    }
  end

build do
  license "MIT"
  license_file "./COPYING"

  command "./configure --prefix=#{install_dir}/embedded", env: configure_env
  command "make -j #{workers}", env: configure_env
  command "make -j #{workers} install", env: configure_env
end
