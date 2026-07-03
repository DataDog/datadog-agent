name "python3"

default_version "3.13.14"


relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""

  command "bazel run #{flavor_flag} --//:install_dir=#{install_dir} -- @cpython//:install --destdir=#{install_dir}",
      :live_stream => Omnibus.logger.live_stream(:info)

  if !windows_target?
    # Libraries and binaries are rpath-patched by dd_cc_packaged in cpython.BUILD.bazel;
    # this call is now only for the ##PREFIX## text substitution in _sysconfigdata.
    command "bazel run --//:install_dir=#{install_dir} -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/python3.*/_sysconfigdata__*.py",
      :live_stream => Omnibus.logger.live_stream(:info)
  end
end
