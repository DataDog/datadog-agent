name "datadog-windows-filter-driver"
# at this moment,builds are stored by branch name.  Will need to correct at some point


default_version "master"
#
# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    driverpath = ENV['WINDOWS_DDNPM_DRIVER']
    driverver = ENV['WINDOWS_DDNPM_VERSION']
    drivermsmsha = ENV['WINDOWS_DDNPM_SHASUM']

    source :url => "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driverpath}/ddnpminstall-#{driverver}.msm",
           :sha256 => "#{drivermsmsha}",
           :target_filename => "DDNPM.msm"

    build do
        copy "DDNPM.msm", "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/DDNPM.msm"
    end

end