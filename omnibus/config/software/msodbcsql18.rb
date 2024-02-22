name "msodbcsql18"
default_version "18.3.2.1-1"

dependency "unixodbc"

license "MICROSOFT SOFTWARE LICENSE"
license_file "doc/msodbcsql18/LICENSE.txt"
skip_transitive_dependency_licensing true

# Dynamically build source url and sha256 based on the linux platform and architecture
# The source url and sha256 are taken from the official Microsoft ODBC Driver for SQL Server page:
if debian_target?
  if arm_target?
    # Debian ARM64 build
    package_name = "#{name}_#{version}_arm64.deb"
    source url: "https://packages.microsoft.com/debian/12/prod/pool/main/m/msodbcsql18/#{package_name}"
    source sha256: "d9bb2d2e165e9d86f6b5de75b2baa24c9f0f25107471bcf82254d27f5dafff30"
  else
    # Debian AMD64 build
    package_name = "#{name}_#{version}_amd64.deb"
    source url: "https://packages.microsoft.com/debian/12/prod/pool/main/m/msodbcsql18/#{package_name}"
    source sha256: "ae8eea58236e46c3f4eae05823cf7f0531ac58f12d90bc24245830b847c052ee"
  end
elsif redhat_target?
  package_name = "#{name}-#{version}.x86_64.rpm"
  source url: "https://packages.microsoft.com/rhel/8/prod/Packages/m/#{package_name}"
  source sha256: "ecd8e148138ee72a452a5357e380580e2c19219c5424f4ac9350cbdf3217fad1"
else
  package_name = "#{name}-#{version}.x86_64.rpm"
  source url: "https://packages.microsoft.com/sles/15/prod/Packages/m/#{package_name}"
  source sha256: "e8b753f8730681d9308f5e3e9e2bde4e169d5e598e322a7d6e31860b73af55e6"
end

relative_path "msodbcsql18-#{version}"

build do
  command "mkdir -p #{project_dir}/#{relative_path}"
  command "mkdir -p #{install_dir}/embedded/msodbcsql/lib"
  if debian_target?
    command "dpkg-deb -R #{project_dir}/#{name}-#{package_name} #{project_dir}/#{relative_path}"
  else
    command "rpm2cpio #{project_dir}/#{name}-#{package_name} | (cd #{project_dir}/#{relative_path} && cpio -idmv)"
  end
  # Fix rpath first
  command "patchelf --force-rpath --set-rpath '#{install_dir}/embedded/lib' '#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.3.so.2.1'"
  # Manually move the files we need and ensure the symlink aren't broken
  move "#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/*", "#{install_dir}/embedded/msodbcsql/", :force => true
  link "#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.2.1", "#{install_dir}/embedded/msodbcsql/lib/libmsodbcsql-18.3.so"
  # Also move the license bits
  move "#{project_dir}/#{relative_path}/usr/share", "#{install_dir}/embedded/msodbcsql/", :force => true
end
