require 'ci/common'

def linux_proc_extras_version
  ENV['FLAVOR_VERSION'] || 'latest'
end

def linux_proc_extras_rootdir
  "#{ENV['INTEGRATIONS_DIR']}/linux_proc_extras_#{linux_proc_extras_version}"
end

namespace :ci do
  namespace :linux_proc_extras do |flavor|
    task before_install: ['ci:common:before_install']

    task :install do
      Rake::Task['ci:common:install'].invoke('linux_proc_extras')
      # sample docker usage
      # sh %(docker create -p XXX:YYY --name linux_proc_extras source/linux_proc_extras:linux_proc_extras_version)
      # sh %(docker start linux_proc_extras)
    end

    task before_script: ['ci:common:before_script']

    task script: ['ci:common:script'] do
      this_provides = [
        'linux_proc_extras'
      ]
      Rake::Task['ci:common:run_tests'].invoke(this_provides)
    end

    task before_cache: ['ci:common:before_cache']

    task cleanup: ['ci:common:cleanup']
    # sample cleanup task
    # task cleanup: ['ci:common:cleanup'] do
    #   sh %(docker stop linux_proc_extras)
    #   sh %(docker rm linux_proc_extras)
    # end

    task :execute do
      exception = nil
      begin
        %w(before_install install before_script).each do |u|
          Rake::Task["#{flavor.scope.path}:#{u}"].invoke
        end
        if !ENV['SKIP_TEST']
          Rake::Task["#{flavor.scope.path}:script"].invoke
        else
          puts 'Skipping tests'.yellow
        end
        Rake::Task["#{flavor.scope.path}:before_cache"].invoke
      rescue => e
        exception = e
        puts "Failed task: #{e.class} #{e.message}".red
      end
      if ENV['SKIP_CLEANUP']
        puts 'Skipping cleanup, disposable environments are great'.yellow
      else
        puts 'Cleaning up'
        Rake::Task["#{flavor.scope.path}:cleanup"].invoke
      end
      raise exception if exception
    end
  end
end
