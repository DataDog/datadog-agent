name "msodbcsql18"
default_version "18.3.2.1-1"

dependency "unixodbc"

# Dynamically build source url and sha256 based on the linux platform and architecture
# The source url and sha256 are taken from the official Microsoft ODBC Driver for SQL Server page:

if debian_target?
  source url: "https://packages.microsoft.com/debian/12/prod/pool/main/m/msodbcsql18/#{name}_#{version}_amd64.deb"
  source sha256: "ae8eea58236e46c3f4eae05823cf7f0531ac58f12d90bc24245830b847c052ee"
elsif redhat_target?
  source url: "https://packages.microsoft.com/rhel/8/prod/Packages/m/#{name}-#{version}.x86_64.rpm"
  source sha256: "ecd8e148138ee72a452a5357e380580e2c19219c5424f4ac9350cbdf3217fad1"
else
  source url: "https://packages.microsoft.com/sles/15/prod/Packages/m/#{name}-#{version}.x86_64.rpm"
  source sha256: "e8b753f8730681d9308f5e3e9e2bde4e169d5e598e322a7d6e31860b73af55e6"
end

relative_path "msodbcsql18-#{version}"

build do
  if !arm_target?
    command "mkdir -p #{install_dir}/embedded/msodbcsql/lib"
    if debian_target?
      command "dpkg-deb -R #{project_dir}/#{name}-#{name}_#{version}_amd64.deb #{project_dir}/#{relative_path}"
    else
      command "rpm2cpio #{project_dir}/#{name}-#{version}.x86_64.rpm | (cd #{project_dir}/#{relative_path} && cpio -idmv)"
    end
    # Fix rpath first
    command "patchelf --force-rpath --set-rpath '#{install_dir}/embedded/lib' '#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.3.so.2.1'"
    # Manually move the files we need and ensure the symlink aren't broken
    move "#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/*", "#{install_dir}/embedded/msodbcsql/", :force => true
    link "#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.2.1", "#{install_dir}/embedded/msodbcsql/lib/libmsodbcsql-18.3.so"
    # Also move the license bits
    move "#{project_dir}/#{relative_path}/usr/share", "#{install_dir}/embedded/msodbcsql/", :force => true
  end
end
