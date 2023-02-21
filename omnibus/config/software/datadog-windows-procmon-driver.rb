name "datadog-windows-procmon-driver"
# at this moment,builds are stored by branch name.  Will need to correct at some point


default_version "master"
#
# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    driverpath = ENV['WINDOWS_DDPROCMON_DRIVER']
    driverver = ENV['WINDOWS_DDPROCMON_VERSION']
    drivermsmsha = ENV['WINDOWS_DDPROCMON_SHASUM']

    source :url => "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driverpath}/ddprocmoninstall-#{driverver}.msm",
           :sha256 => "#{drivermsmsha}",
           :target_filename => "DDPROCMON.msm"

    build do
        copy "DDPROCMON.msm", "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/DDPROCMON.msm"
    end

end