name 'existing-agent-package'

description 'A previously built artifact, unpacked'

always_build true

dependency 'systemd'

source_url = ENV['OMNIBUS_REPACKAGE_SOURCE_URL']
target_package = File.basename(source_url)
source url: source_url,
       sha256: ENV['OMNIBUS_REPACKAGE_SOURCE_SHA256'],
       target_filename: target_package

build do
  command "dpkg --unpack #{target_package}"
end
