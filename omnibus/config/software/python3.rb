name "python3"

default_version "3.13.12"

dependency "openssl3"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""

  if !windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    command_on_repo_root "bazelisk run -- @cpython//:install --destdir='#{install_dir}'"
    sh_ext = if linux_target? then "so" else "dylib" end
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/libpython3.*#{sh_ext}" \
      " #{install_dir}/embedded/lib/python3.13/lib-dynload/*.so" \
      " #{install_dir}/embedded/bin/python3*"
  else
    command_on_repo_root "bazelisk run #{flavor_flag} -- @cpython//:install --destdir=#{install_dir}"
  end
end
