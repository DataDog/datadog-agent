name "msodbcsql18"
default_version "18.3.3.1-1"

dependency "libkrb5"
dependency "unixodbc"

license "MICROSOFT SOFTWARE LICENSE"
license_file "doc/msodbcsql18/LICENSE.txt"
skip_transitive_dependency_licensing true

# Dynamically build source url and sha256 based on the linux platform and architecture
# The source url and sha256 are taken from the official Microsoft ODBC Driver for SQL Server page:
source_url_base = "https://packages.microsoft.com"

if debian_target?
  base_package_name = "#{name}_#{version}"
  if arm_target?
    # Debian ARM64 build
    package_name = "#{base_package_name}_arm64.deb"
    source sha256: "37e692b1517f1229042c743d0f2a7191e0dcb956bbc3785a895aaa6dc328467e"
    source url: "#{source_url_base}/debian/12/prod/pool/main/m/msodbcsql18/#{package_name}"
  else
    # Debian AMD64 build
    package_name = "#{base_package_name}_amd64.deb"
    source sha256: "f91004ce72fcd92e686154f90e2a80f4f86469e7cb5f42ef79cba79dc6727890"
    source url: "#{source_url_base}/debian/12/prod/pool/main/m/msodbcsql18/#{package_name}"
  end
elsif redhat_target?
  base_package_name = "#{name}-#{version}"
  if arm_target?
    # RHEL aarch64 build
    package_name = "#{base_package_name}.aarch64.rpm"
    source sha256: "ffbdede1d6af1245ce02cb77d3d5d37648b9121900632d3c6de56fe60d436b12"
    source url: "#{source_url_base}/rhel/8/prod/Packages/m/#{package_name}"
  else
    # RHEL x86_64 build
    package_name = "#{base_package_name}.x86_64.rpm"
    source sha256: "8914b1e64c37f3740dff96f7e5db57f40f9564979617ad38acf3d38b958b7cb7"
    source url: "#{source_url_base}/rhel/8/prod/Packages/m/#{package_name}"
  end
else
  # SLES x86_64 build
  package_name = "#{name}-#{version}.x86_64.rpm"
  source sha256: "fc25b9f52aef7a8814667045185103ab967f02301d875d4236e9a6c1493b298a"
  source url: "#{source_url_base}/sles/15/prod/Packages/m/#{package_name}"
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
  command "patchelf --force-rpath --set-rpath '#{install_dir}/embedded/lib' '#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.3.so.3.1'"
  # Manually move the files we need and ensure the symlink aren't broken
  move "#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/*", "#{install_dir}/embedded/msodbcsql/", :force => true
  link "#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1", "#{install_dir}/embedded/msodbcsql/lib/libmsodbcsql-18.3.so"
  # Also move the license bits
  move "#{project_dir}/#{relative_path}/usr/share", "#{install_dir}/embedded/msodbcsql/", :force => true
end
