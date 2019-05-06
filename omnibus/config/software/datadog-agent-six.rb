name "datadog-agent-six"
default_version "master"

dependency "python2"
dependency "python3"

license "Apache"
license_file "LICENSE"
skip_transitive_dependency_licensing true

source path: '../six'

if ohai["platform"] != "windows"
  build do
    env = {
        "Python2_ROOT_DIR" => "#{install_dir}/embedded",
        "Python3_ROOT_DIR" => "#{install_dir}/embedded",
        "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
    }

    command "inv -e six.build --install-prefix \"#{install_dir}/embedded\" --cmake-options '-DCMAKE_CXX_FLAGS:=\"-D_GLIBCXX_USE_CXX11_ABI=0\" -DCMAKE_FIND_FRAMEWORK:STRING=NEVER'", :env => env
    command "inv -e six.install"
  end
else
  build do
    env = {
        "Python2_ROOT_DIR" => "#{windows_safe_path(python_2_embedded)}",
        "Python3_ROOT_DIR" => "#{windows_safe_path(python_3_embedded)}",
        "CMAKE_INSTALL_PREFIX" => "#{windows_safe_path(python_2_embedded)}"
    }

    command "inv -e six.build --install-prefix \"#{windows_safe_path(python_2_embedded)}\" --cmake-options '-G \"Unix Makefiles\"", :env => env
    command "mv bin/*.dll  #{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/"

  end
end
