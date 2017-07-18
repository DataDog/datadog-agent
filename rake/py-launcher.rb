require_relative './common'

namespace :pylauncher do
  PYLAUNCHER_BIN_PATH="./bin/py-launcher"
  CLOBBER.include(PYLAUNCHER_BIN_PATH)

  desc "Build py-launcher [incremental=false]"
  task :build do
    build_type_opt = ENV['incremental'] == "true" ? "-i" : "-a"

    bin_path = PYLAUNCHER_BIN_PATH
    sh("go build #{build_type_opt} -o #{bin_path}/#{bin_name("py-launcher")} #{REPO_PATH}/cmd/py-launcher/")
  end

  desc "Run system tests with pylauncher"
  task :system_test => %w[pylauncher:build] do
    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, PYLAUNCHER_BIN_PATH, "py-launcher")
    sh("cd #{root}/test/system/python_binding/ && PYLAUNCHER_BIN=\"#{bin_path}\" ./test.sh")
  end
end
