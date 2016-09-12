name 'datadog-agent'

source path: '..'

relative_path 'datadog-agent'

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  command "rake build"
  command "cp -Rf bin #{install_dir}/"
end
