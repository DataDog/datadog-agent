require "./lib/packagers/deb.rb"
require "./lib/packagers/msi.rb"
require "./lib/packagers/rpm.rb"
require "./lib/packagers/zip.rb"

module Omnibus
  module Packager

    #
    # The list of Ohai platform families mapped to the respective packager
    # class.
    #
    # @return [Hash<String, Class>]
    #
    PLATFORM_PACKAGER_MAP = {
      "debian" => DEB,
      "fedora" => RPM,
      "suse" => RPM,
      "rhel" => RPM,
      "wrlinux" => RPM,
      "amazon" => RPM,
      "aix" => BFF,
      "solaris" => Solaris,
      "omnios" => IPS,
      "ips" => IPS,
      "windows" => [MSI, ZIP],
      "mac_os_x" => PKG,
      "smartos" => PKGSRC,
    }.freeze
  end
end
