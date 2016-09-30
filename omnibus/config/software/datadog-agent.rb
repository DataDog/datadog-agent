name 'datadog-agent'

dependency 'python'

source path: '..'

relative_path 'datadog-agent'

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  command 'rake build'
  copy('bin', install_dir)
end
