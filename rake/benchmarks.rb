require_relative './common'

namespace :benchmark do
  BENCHMARK_BIN_PATH="./bin/benchmarks"
  CLOBBER.include(BENCHMARK_BIN_PATH)

  namespace :aggregator do
    desc "Build the aggregator benchmark"
    task :build do
      # -race option
      build_type = ENV['incremental'] == "true" ? "-i" : "-a"
      flags = ""

      if ENV["WINDOWS_DELVE"]
        # On windows, need to build with the extra arguments -gcflags "-N -l" -ldflags="-linkmode internal"
        # if you want to be able to use the delve debugger.
        flags="-gcflags \"-N -l\" -ldflags=\"-linkmode internal\""
      end

      system("go build #{build_type} -tags '#{go_build_tags}' -o #{BENCHMARK_BIN_PATH}/#{bin_name("aggregator")} #{flags} #{REPO_PATH}/test/benchmarks/aggregator") or exit!(1)
    end

    desc "Run the aggregator benchmark"
    task :run do
      branch = ENV["DD_REPO_BRANCH_NAME"] or `git rev-parse --abbrev-ref HEAD`.strip()
      options = ""
      if ENV["DD_AGENT_API_KEY"]
        options = " -api-key #{ENV["DD_AGENT_API_KEY"]}"
      end

      system("#{BENCHMARK_BIN_PATH}/#{bin_name("aggregator")} -points 2,10,100,500,1000 -series 10,100,1000 -log-level info -json -branch #{branch} #{options}") or exit!(1)
    end
  end

  namespace :dogstatsd do
    desc "Build Dogstatsd benchmark"
    task :build do
      system("go build -tags '#{go_build_tags}' -o #{BENCHMARK_BIN_PATH}/#{bin_name("dogstatsd")} #{REPO_PATH}/test/benchmarks/dogstatsd") or exit!(1)
    end

    desc "Run Dogstatsd Benchmark"
    task :run => %w[benchmark:dogstatsd:build] do
      root = `git rev-parse --show-toplevel`.strip
      bin_path = File.join(root, BENCHMARK_BIN_PATH, "dogstatsd")
      system("#{bin_path} -pps=5000 -dur 45 -ser 5 -brk -inc 1000")
    end
  end
end
