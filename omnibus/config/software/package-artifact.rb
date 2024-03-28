name 'package-artifacts'

description "Helper to package an XZ build artifact to deb/rpm/..."

build do
  command "tar xf #{ENV['OMNIBUS_PACKAGE_ARTIFACT']} -C /"
  delete "#{ENV['OMNIBUS_PACKAGE_ARTIFACT']}"
end

