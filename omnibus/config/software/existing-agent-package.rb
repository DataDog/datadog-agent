name 'existing-agent-package'

description 'A previously built artifact, unpacked'

require 'fileutils'
require 'shellwords'

always_build true

source_url = ENV['OMNIBUS_REPACKAGE_SOURCE_URL']
target_package = File.basename(source_url)
source url: source_url,
       sha256: ENV['OMNIBUS_REPACKAGE_SOURCE_SHA256'],
       target_filename: target_package

build do
  destdir = ENV["OMNIBUS_BASE_DIR"] || "/"

  block "Prepare package extraction root" do
    FileUtils.mkdir_p(destdir)
  end

  command "dpkg-deb -x #{Shellwords.escape(target_package)} #{Shellwords.escape(destdir)}"

  if destdir != "/"
    staged_install_dir = File.join(destdir, install_dir.sub(%r{\A/+}, ""))

    block "Populate install directory from extracted package" do
      FileUtils.mkdir_p(install_dir)
      FileUtils.cp_r("#{staged_install_dir}/.", install_dir, preserve: true)
    end
  end
end
