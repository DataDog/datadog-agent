name "python3"

default_version "3.13.14"


relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  command "bazel run #{omnibazel_flags} -- @cpython//:install --destdir=#{install_dir}",
      :live_stream => Omnibus.logger.live_stream(:info)
end
