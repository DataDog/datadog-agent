name "msodbcsql18"
default_version "18.3.2.1-1"

dependency "unixodbc"

if arm_target? 
  arch "arm64"
  source sha256: "d9bb2d2e165e9d86f6b5de75b2baa24c9f0f25107471bcf82254d27f5dafff30"
else
  arch "amd64"
  source sha256: "ae8eea58236e46c3f4eae05823cf7f0531ac58f12d90bc24245830b847c052ee"
end

source url: "https://packages.microsoft.com/debian/12/prod/pool/main/m/msodbcsql18/#{name}_#{version}_#{arch}.deb"

relative_path "msodbcsql18-#{version}"

build do
  if debian_target?
    command "mkdir -p #{install_dir}/embedded/msodbcsql/lib"
    command "dpkg-deb -R #{project_dir}/#{name}-#{name}_#{version}_#{arch}.deb #{project_dir}/#{relative_path}"
    # Fix rpath first
    command "patchelf --force-rpath --set-rpath '#{install_dir}/embedded/lib' '#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.3.so.2.1'"
    # Manually move the files we need and ensure the symlink aren't broken
    move "#{project_dir}/#{relative_path}/opt/microsoft/msodbcsql18/*", "#{install_dir}/embedded/msodbcsql/", :force => true
    link "#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.2.1", "#{install_dir}/embedded/msodbcsql/lib/libmsodbcsql-18.3.so"
    # Also move the license bits
    move "#{project_dir}/#{relative_path}/usr/share", "#{install_dir}/embedded/msodbcsql/", :force => true
  end
end
