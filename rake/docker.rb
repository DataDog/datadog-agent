require_relative './common'

namespace :docker do
  DOGSTATSD_TAG="datadog/dogstatsd:master"

  desc "Build datadog/dogstatsd docker image"
  task :build_dogstatsd do
    if ENV['skip_rebuild'] == "true" then
      puts "Skipping DogStatsD build"
    else
      puts "Building DogStatsD"
      Rake::Task["dogstatsd:build_static"].invoke
    end
    FileUtils.cp("bin/static/dogstatsd", "Dockerfiles/dogstatsd/alpine/")
    system("docker build -t #{DOGSTATSD_TAG} Dockerfiles/dogstatsd/alpine/") || exit(1)
  end

  desc "Run docker integration tests"
  task :integration_test => %w[docker:build_dogstatsd]  do
    puts "Starting docker integration tests"
    system("DOCKER_IMAGE=#{DOGSTATSD_TAG} ./test/integration/docker/dsd_alpine_listening.sh") || exit(1)
  end

end
