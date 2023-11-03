name "msodbcsql18"
default_version "18.3.2.1-1"

dependency "unixodbc"

source url: "https://packages.microsoft.com/debian/12/prod/pool/main/m/msodbcsql18/#{name}_#{version}_amd64.deb"
source sha256: "ae8eea58236e46c3f4eae05823cf7f0531ac58f12d90bc24245830b847c052ee"

relative_path "msodbcsql18-#{version}"

build do
  if debian_target? && !arm_target?
    command "mkdir -p #{install_dir}/embedded/msodbcsql"
    command "dpkg-deb -R #{project_dir}/#{name}-#{name}_#{version}_amd64.deb #{project_dir}/#{relative_path}"
    move "#{project_dir}/#{relative_path}/*", "#{install_dir}/embedded/msodbcsql/", :force=>true
  end
end
