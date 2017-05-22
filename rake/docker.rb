require_relative './common'

namespace :docker do

  desc "Run docker integration tests"
  task :integration_test  do
    if ENV['skip_rebuild'] == "true" then
      puts "Skipping DogStatsD build"
    else
      puts "Building DogStatsD static binary"
      Rake::Task["dogstatsd:build_static"].invoke
    end
    puts "Starting docker integration tests"
    system("bash test/integration/docker/dsd_alpine_listening.sh") || exit(1)
  end

end
