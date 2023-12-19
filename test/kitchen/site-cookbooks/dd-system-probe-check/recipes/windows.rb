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
  not_if { ::File.directory?(::File.join(tmp_dir, 'wix')) }
end

cookbook_file "#{tmp_dir}\\decompress_merge_module.ps1" do
  source 'decompress_merge_module.ps1'
end

execute 'extract driver merge module' do
  cwd tmp_dir
  live_stream true
  environment 'WIX' => "#{tmp_dir}\\wix"
  command "powershell -C \".\\decompress_merge_module.ps1 -file ddnpm.msm -targetDir .\\expanded\""
  not_if { ::File.exist?(::File.join(tmp_dir, 'expanded', 'ddnpm.msm')) }
end

execute 'IIS' do
  command "powershell -C \"Install-WindowsFeature -name Web-Server -IncludeManagementTools\""
end

directory "Make IIS Paths" do 
  path "c:\\tmp\\inetpub\\testsite1"
  recursive true
end

cookbook_file "c:\\tmp\\inetpub\\testsite1\\iisstart.htm" do
  source 'iisstart.htm'
end
cookbook_file "c:\\tmp\\inetpub\\testsite1\\iisstart.png" do
  source 'iisstart.png'
end

directory "Make IIS Paths" do 
  path "c:\\tmp\\inetpub\\testsite2"
  recursive true
end

cookbook_file "c:\\tmp\\inetpub\\testsite2\\iisstart.htm" do
  source 'iisstart.htm'
end
cookbook_file "c:\\tmp\\inetpub\\testsite2\\iisstart.png" do
  source 'iisstart.png'
end
execute "create testsite 1" do
  command "powershell -C \"New-IISSite -Name 'TestSite1' -BindingInformation '*:8081:' -PhysicalPath c:\\tmp\\inetpub\\testsite1\""
end

execute "create testsite 2" do
  command "powershell -C \"New-IISSite -Name 'TestSite2' -BindingInformation '*:8082:' -PhysicalPath c:\\tmp\\inetpub\\testsite2\""
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

execute 'system-probe-driver-install' do
  command "powershell -C \"sc.exe create ddnpm type= kernel binpath= #{tmp_dir}\\expanded\\ddnpm.sys start= demand\""
  not_if 'sc.exe query ddnpm'
end

windows_service 'system-probe-driver' do
  service_name 'ddnpm'
  action :start
end
