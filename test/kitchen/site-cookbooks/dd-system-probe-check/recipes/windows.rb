require 'json'

# manually install and start the NPM driver
tmp_dir = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp"
dna_json_path = "#{tmp_dir}\\kitchen\\dna.json"
agentvars = JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec')
driver_path = agentvars.fetch('driver_path')
driver_ver = agentvars.fetch('driver_ver')
driver_msmsha = agentvars.fetch('driver_msmsha')

remote_path = "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driver_path}/ddnpminstall-#{driver_ver}.msm"
remote_file "#{tmp_dir}\\ddnpm.msm" do
  source remote_path
  checksum driver_msmsha
end

remote_file "#{tmp_dir}\\wix311-binaries.zip" do
  source "https://github.com/wixtoolset/wix3/releases/download/wix3112rtm/wix311-binaries.zip"
end

execute 'wix-extract' do
  cwd tmp_dir
  command "powershell -C \"Add-Type -A 'System.IO.Compression.FileSystem'; [IO.Compression.ZipFile]::ExtractToDirectory('wix311-binaries.zip', 'wix');\""
end

cookbook_file "#{tmp_dir}\\decompress_merge_module.ps1" do
  source 'decompress_merge_module.ps1'
end

execute 'extract driver merge module' do
  cwd tmp_dir
  live_stream true
  environment 'WIX' => "#{tmp_dir}\\wix"
  command "powershell -C \".\\decompress_merge_module.ps1 -file ddnpm.msm -targetDir .\\expanded\""
end

execute 'system-probe-driver-install' do
  command "powershell -C \"sc.exe create ddnpm type= kernel binpath= #{tmp_dir}\\expanded\\ddnpm.sys start= demand\""
end
# windows_service 'system-probe-driver-install' do
#   service_name 'ddnpm'
#   action :create
#   binary_path_name "#{tmp_dir}\\expanded\\ddnpm.sys"
#   startup_type :manual
#   service_type 1 # kernel type
# end

windows_service 'system-probe-driver' do
  service_name 'ddnpm'
  action :start
end
