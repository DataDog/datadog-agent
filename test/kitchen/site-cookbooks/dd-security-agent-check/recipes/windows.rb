require 'json'

rootdir = value_for_platform(
  'windows' => { 'default' => ::File.join(Chef::Config[:file_cache_path], 'security-agent') },
  'default' => '/tmp/ci/system-probe'
)

directory "#{rootdir}/tests" do
  recursive true
end
  
directory "#{rootdir}/tests/etw" do
  recursive true
end


cookbook_file "#{rootdir}/tests/testsuite.exe" do
  source "tests/testsuite.exe"
  mode '755'
end

cookbook_file "#{rootdir}/tests/etw/testsuite.exe" do
  source "tests/etw/testsuite.exe"
  mode '755'
end

# manually install and start the procmon driver
tmp_dir = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp"
dna_json_path = "#{tmp_dir}\\kitchen\\dna.json"
agentvars = JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec')
driver_path = agentvars.fetch('driver_path')
driver_ver = agentvars.fetch('driver_ver')
driver_msmsha = agentvars.fetch('driver_msmsha')

remote_path = "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driver_path}/ddprocmoninstall-#{driver_ver}.msm"
remote_file "#{tmp_dir}\\ddprocmon.msm" do
  source remote_path
  checksum driver_msmsha
end

remote_file "#{tmp_dir}\\wix311-binaries.zip" do
  source "https://github.com/wixtoolset/wix3/releases/download/wix3112rtm/wix311-binaries.zip"
end


execute 'wix-extract' do
  cwd tmp_dir
  command "powershell -C \"Add-Type -A 'System.IO.Compression.FileSystem'; [IO.Compression.ZipFile]::ExtractToDirectory('wix311-binaries.zip', 'wix');\""
  not_if { ::File.directory?(::File.join(tmp_dir, 'wix')) }
end

cookbook_file "#{tmp_dir}\\decompress_merge_module.ps1" do
  source 'decompress_merge_module.ps1'
end

execute 'extract driver merge module' do
  cwd tmp_dir
  live_stream true
  environment 'WIX' => "#{tmp_dir}\\wix"
  command "powershell -C \".\\decompress_merge_module.ps1 -file ddprocmon.msm -targetDir .\\expanded\""
  not_if { ::File.exist?(::File.join(tmp_dir, 'expanded', 'ddprocmon.msm')) }
end

if driver_path == "testsigned"
  reboot 'now' do
    action :nothing
    reason 'Cannot continue Chef run without a reboot.'
  end

  execute 'enable unsigned drivers' do
    command "bcdedit.exe /set testsigning on"
    notifies :reboot_now, 'reboot[now]', :immediately
    not_if 'bcdedit.exe | findstr "testsigning" | findstr "Yes"'
  end
end

execute 'procmon-driver-install' do
  command "powershell -C \"sc.exe create ddprocmon type= kernel binpath= #{tmp_dir}\\expanded\\ddprocmon.sys start= demand\""
  not_if 'sc.exe query ddprocmon'
end

windows_service 'procmon-driver' do
  service_name 'ddprocmon'
  action :start
end
