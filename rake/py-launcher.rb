require_relative './common'

namespace :pylauncher do
  PYLAUNCHER_BIN_PATH="./bin/py-launcher"
  CLOBBER.include(PYLAUNCHER_BIN_PATH)

  desc "Build py-launcher [incremental=false]"
  task :build do
    # Check if we should use Embedded or System Python,
    # default to the embedded one.
    env = {}
    gcflags = []
    ldflags = []

    if !ENV["USE_SYSTEM_LIBS"]
      env["PKG_CONFIG_PATH"] = "#{PKG_CONFIG_EMBEDDED_PATH}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
      ENV["PKG_CONFIG_PATH"] = "#{PKG_CONFIG_EMBEDDED_PATH}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
      libdir = `pkg-config --variable=libdir python-2.7`.strip
      fail "Can't find path to embedded lib directory with pkg-config" if libdir.empty?
      ldflags << "-r #{libdir}"
    else
      if os == "windows"
        env["PKG_CONFIG_PATH"] = "#{ENV["PKG_CONFIG_SYSTEM"]}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
        ENV["PKG_CONFIG_PATH"] = "#{ENV["PKG_CONFIG_SYSTEM"]}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
        libdir = `pkg-config --variable=libdir python-2.7`.strip
        fail "Can't find path to embedded lib directory with pkg-config" if libdir.empty?
        ldflags << "-r #{libdir}"
      end
    end
    build_type_opt = ENV['incremental'] == "true" ? "-i" : "-a"

    build_tags = go_build_tags

    bin_path = PYLAUNCHER_BIN_PATH
    sh("go build #{build_type_opt} -tags \"#{go_build_tags}\" -o #{bin_path}/#{bin_name("py-launcher")} #{REPO_PATH}/cmd/py-launcher/")
  end

  desc "Run system tests with pylauncher"
  task :system_test => %w[pylauncher:build] do
    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, PYLAUNCHER_BIN_PATH, "py-launcher")
    sh("cd #{root}/test/system/python_binding/ && PYLAUNCHER_BIN=\"#{bin_path}\" ./test.sh")
  end
end
