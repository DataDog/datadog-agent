name "python3"

default_version "3.13.12"

unless windows?
  dependency "zlib"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
end
dependency "openssl3"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "12e7cb170ad2d1a69aee96a1cc7fc8de5b1e97a2bdac51683a3db016ec9a2996"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""

  if !windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    command_on_repo_root "bazelisk run -- @cpython//:install --destdir='#{install_dir}/embedded'"
    sh_ext = if linux_target? then "so" else "dylib" end
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/libpython3.*#{sh_ext}" \
      " #{install_dir}/embedded/lib/python3.13/lib-dynload/*.so" \
      " #{install_dir}/embedded/bin/python3*"
  else
    command_on_repo_root "bazelisk run #{flavor_flag} -- @cpython//:install --destdir=#{python_3_embedded}"
  end
end

