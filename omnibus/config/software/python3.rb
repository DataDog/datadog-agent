name "python3"

default_version "3.13.13"


relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""

  if !windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- @cpython//:install --destdir='#{install_dir}'"
    # Libraries and binaries are rpath-patched by dd_cc_packaged in cpython.BUILD.bazel;
    # this call is now only for the ##PREFIX## text substitution in _sysconfigdata.
    command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/python3.*/_sysconfigdata__*.py"
    python = "#{install_dir}/embedded/bin/python3"
  else
    command_on_repo_root "bazelisk run #{flavor_flag} --//:install_dir=#{install_dir} -- @cpython//:install --destdir=#{install_dir}"
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  end

  # Upgrade pip to 26.0.1 to address CVE-2026-1703 (path traversal in pip < 26.0
  # when installing malicious wheel archives). Python 3.13 ships with pip 25.3 via
  # ensurepip, which is vulnerable.
  command "#{python} -m pip install pip==26.0.1"
end
