name "conda"
description "none provided"
default_version "4.2.12"
whitelist_file "/embedded/miniconda/"

ext = "sh"
if ohai["platform"] == "mac_os_x"
    target = "Miniconda2-#{version}-MacOSX-x86_64.sh"
    sum = "ff3d7b69e32e1e4246176fb90f8480c8"
elsif ohai["platform"] == "windows"
    target = "Miniconda2-#{version}-Windows-x86_64.exe"
    sum = "f78d2e149d017c3ccc691fb0586d57e4"
else
    target = "Miniconda2-#{version}-Linux-x86_64.sh"
    sum = "c8b836baaa4ff89192947e4b1a70b07e"
end

source :url => "https://repo.continuum.io/miniconda/#{target}",       
       :md5 => "#{sum}"

build do 
    if ohai["platform"] == "windows"
        command "#{target} /InstallationType=JustMe /RegisterPython=0 /S /D=#{install_dir}/embedded/miniconda"
    else
        command "bash #{target} -b -f -p #{install_dir}/embedded/miniconda"
    end
end
