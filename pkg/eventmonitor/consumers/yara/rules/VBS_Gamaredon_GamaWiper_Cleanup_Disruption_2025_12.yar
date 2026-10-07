rule VBS_Gamaredon_GamaWiper_Cleanup_Disruption_2025_12
{
  meta:
    description = "Detects VBScript cleanup/disruption tool wiping HKCU persistence, deleting C:\\Users recursively, deleting scheduled tasks, and killing script processes"
    author = "Robin Dost"
    date = "2025-12-23"
    reference = "User-provided script"
    confidence = "high"
    tags = "vbs, wscript, cleanup, disruption, persistence-removal"
    yarahub_author_twitter = "@Mr128BitSec"
    yarahub_author_email = "robin.dost@synapticsystems.de"
    yarahub_reference_md5 = "4de669a86175e24bcd26c451240b6fa0"
    yarahub_uuid = "96b89a92-1d5f-4cc5-b606-64963117c4fa"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"

  strings:
    // VBScript / COM primitives
    $wshell = "WScript.Shell" ascii wide
    $fso    = "Scripting.FileSystemObject" ascii wide
    $stdreg = "StdRegProv" ascii wide
    $wmi1   = "winmgmts:\\\\.\\root\\default:StdRegProv" ascii wide
    $wmi2   = "root\\cimv2" ascii wide
    $query  = "Select * from Win32_Process Where Name = "

    // Targeted persistence keys
    $run    = "HKCU\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run\\" ascii wide
    $runonce= "HKCU\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\RunOnce\\" ascii wide
    $impersonate = "impersonationLevel=impersonate"
    // Scheduled task wipe
    $scht   = "cmd /c schtasks /delete /tn * /f" ascii wide

    // User directory wipe
    $users  = "C:\\Users" ascii wide
    $delrec = "DeleteFilesRecursively" ascii wide
    $filedel= "file.Delete True" ascii wide

    // Registry walk + delete
    $walk   = "WalkRegistry" ascii wide
    $regread= "objShell.RegRead" ascii wide
    $regdel = "objShell.RegDelete" ascii wide
    $hkcu   = "&H80000001" ascii wide

    // Process kill via WMI
    $q_ps   = "Select * from Win32_Process Where Name = 'powershell.exe'" ascii wide
    $q_ws   = "Select * from Win32_Process Where Name = 'wscript.exe'" ascii wide
    $q_cs   = "Select * from Win32_Process Where Name = 'cscript.exe'" ascii wide
    $q_ms   = "Select * from Win32_Process Where Name = 'mshta.exe'" ascii wide
    $term   = ".Terminate()" ascii wide

  condition:
    // Ensure it's a VBS script-ish file plus the unique behavior combo
    (
      $wshell and $regdel and $query and $impersonate and
      $fso and $stdreg and $wmi1 and
      ( $run or $runonce ) and
      $scht and
      $users and $delrec and $filedel and
      $wmi2 and $term and
      2 of ($q_ps, $q_ws, $q_cs, $q_ms) and
      ( $walk and $regread and $hkcu )
    )
}
