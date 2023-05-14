# Microsoft Visual C++ redistributable

name "vc_redist"
default_version "90"

# source :url => "https://dd-agent-omnibus.s3.amazonaws.com/msvcrntm_x64.tar.gz",
if windows_arch_i386?
  source :url => "https://dd-agent-omnibus.s3.amazonaws.com/msvc_runtime_x86.tgz",
         :sha256 => "6fee9db533c6547648ea8423d9a0a281298586c3a0761d17ba3f36c5360c2434",
         :extract => :seven_zip
else
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/msvc_runtime_x64.tgz",
           :sha256 => "ee3d4be86e7a63a7a9f9f325962fcf62436ac234f1fd69919003463ffd43ee3f",
           :extract => :seven_zip

end

build do
  license "Microsoft Visual Studio 2008"
  license_file "https://s3.amazonaws.com/dd-agent-omnibus/omnibus/vcredist_90_license.pdf"

  # Because python is built with really old (VS2008) visual c++, and with the CRT
  # as a DLL, we need to redistribute the CRT DLLS.  We (now) need the DLLS in
  # both embedded and dist, as we have executables in each of those directories
  # that require them.
  command "XCOPY /YEH .\\*.* \"#{windows_safe_path(python_2_embedded)}\" /IR"

  #
  # also copy them to the bin/agent directory, so we can (optionally) install on
  # 2008.
  copy '*.dll', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/"
  copy '*.manifest', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/"
end
