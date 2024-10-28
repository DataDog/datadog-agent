# Microsoft Visual C++ redistributable

name "vc_ucrt_redist"
default_version "14"

# source :url => "https://dd-agent-omnibus.s3.amazonaws.com/msvcrntm_x64.tar.gz",
if windows_arch_i386?
  source :url => "https://dd-agent-omnibus.s3.amazonaws.com/msvc_ucrt_runtime_x86.zip",
         :sha256 => "7f93f888b3f99f890557a7361381b10552022744422e482510c24f25cf09191d",
         :extract => :seven_zip

  build do
    # Because python is built with really old (VS2008) visual c++, and with the CRT
    # as a DLL, we need to redistribute the CRT DLLS.  We (now) need the DLLS in
    # both embedded and dist, as we have executables in each of those directories
    # that require them.
    command "XCOPY /YEH .\\*.* \"#{windows_safe_path(python_3_embedded)}\" /IR"

  end
end