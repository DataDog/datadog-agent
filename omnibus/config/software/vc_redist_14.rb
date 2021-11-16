# Microsoft Visual C++ redistributable

name "vc_redist_140"
default_version "140"

if windows_arch_i386?
  source :url => "https://s3.amazonaws.com/dd-agent-omnibus/Microsoft_VC141_CRT_x86.msm",
         :sha256 => "d582b8069174edaefb1fa04b84ebca375602655b5bdbb17aa15d61f43b25a67e",
         :target_filename => "Microsoft_VC141_CRT_x86.msm"
else
  source :url => "https://s3.amazonaws.com/dd-agent-omnibus/Microsoft_VC141_CRT_x64.msm",
         :sha256 => "102a2127f528865f6e462c5b28589a7249f70d0d4201676c3b2f2cc46f997b84",
         :target_filename => "Microsoft_VC141_CRT_x64.msm"
end
build do
  license "Microsoft Visual Studio 2015"
  license_file "https://s3.amazonaws.com/dd-agent-omnibus/omnibus/vcredist_140_license.txt"

   # expand the MSM so that anyone that needs the individual components can find it
   script_root = "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/tools/windows/decompress_merge_module.ps1"
   if windows_arch_i386?
     source_msm = "Microsoft_VC141_CRT_x86.msm"
   else
     source_msm = "Microsoft_VC141_CRT_x64.msm"
   end
   command "powershell -C \"#{windows_safe_path(script_root)} -file #{source_msm} -targetDir .\\expanded\""
end
