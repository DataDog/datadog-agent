name "datadog-windows-filter-driver"
# at this moment,builds are stored by branch name.  Will need to correct at some point


default_version "master"
#
# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    driverpath = ENV['WINDOWS_DDFILTER_DRIVER']
    driverver = ENV['WINDOWS_DDFILTER_VERSION']
    drivermsmsha = ENV['WINDOWS_DDFILTER_SHASUM']

    source :url => "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driverpath}/ddfilterinstall-#{driverver}.msm",
           :sha256 => "#{drivermsmsha}",
           :target_filename => "ddfilter.msm"

    build do
        copy "ddfilter.msm", "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/ddfilter.msm"
    end

end