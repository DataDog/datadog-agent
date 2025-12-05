name "python3"

default_version "3.13.11"

unless windows?
  dependency "zlib"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
end
dependency "openssl3"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "03cfedbe06ce21bc44ce09245e091a77f2fee9ec9be5c52069048a181300b202"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  if !windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    command_on_repo_root "bazelisk run -- @cpython//:install --destdir='#{install_dir}/embedded'"
    sh_ext = if linux_target? then "so" else "dylib" end
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/pkgconfig/python*.pc" \
      " #{install_dir}/embedded/lib/libpython3.*#{sh_ext}" \
      " #{install_dir}/embedded/lib/python3.13/lib-dynload/*.so" \
      " #{install_dir}/embedded/bin/python3*"
  else
    if ENV["AGENT_FLAVOR"] == "fips"
      fips_flag = "--//:fips_mode=true"
    else
      fips_flag = ""
    end
    command_on_repo_root "bazelisk run -- @cpython//:install --destdir=#{python_3_embedded} #{fips_flag}"
  end
end

