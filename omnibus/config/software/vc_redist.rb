# Microsoft Visual C++ redistributable

name "vc_redist"
default_version "90"

# source :url => "https://s3.amazonaws.com/dd-agent-omnibus/msvcrntm_x64.tar.gz",
source :url => "https://s3.amazonaws.com/dd-agent-omnibus/msvc_runtime_x64.tgz",
       :sha256 => "ee3d4be86e7a63a7a9f9f325962fcf62436ac234f1fd69919003463ffd43ee3f",
       :extract => :seven_zip

build do
  # Because python is built with really old (VS2008) visual c++, and with the CRT
  # as a DLL, we need to redistribute the CRT DLLS.  We (now) need the DLLS in
  # both embedded and dist, as we have executables in each of those directories
  # that require them.
  command "XCOPY /YEH .\\*.* \"#{windows_safe_path(install_dir)}\\embedded\" /IR"

end
