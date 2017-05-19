require_relative './common'

namespace :benchmark do
  BENCHMARK_BIN_PATH="./bin/benchmarks"
  CLOBBER.include(BENCHMARK_BIN_PATH)

  desc "Build the aggregator benchmarks"
  task :aggregator do
    # `incremental` option
    build_type = ENV['incremental'] == "true" ? "-i" : "-a"
    flags = ""

    if ENV["WINDOWS_DELVE"]
      # On windows, need to build with the extra arguments -gcflags "-N -l" -ldflags="-linkmode internal"
      # if you want to be able to use the delve debugger.
      flags="-gcflags \"-N -l\" -ldflags=\"-linkmode internal\""
    end

    system("go build #{build_type} -o #{BENCHMARK_BIN_PATH}/#{bin_name("aggregator")} #{flags} #{REPO_PATH}/test/benchmarks/aggregator") or exit!(1)
  end

  desc "Build Dogstatsd benchmark"
  task :dogstatsd do
    system("go build -o #{BENCHMARK_BIN_PATH}/#{bin_name("dogstatsd_benchmark")} #{REPO_PATH}/cmd/benchmark/dogstatsd/")
  end

  desc "Run Dogstatsd Benchmark"
  task :dogstatsd_benchmark => %w[benchmark:dogstatsd] do
    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, DOGSTATSD_BIN_PATH, "dogstatsd_benchmark")
    system("#{bin_path} -pps=1000 -dur 30 -ser 5 -brk -inc 500")
  end

end
