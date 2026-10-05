rule Runtime_Broker_Variant_1 {
   meta:
      description = "Detecting malicious Runtime Broker"
      author = "Sn0wFr0$t"
      date = "2025-06-01"
      yarahub_uuid = "2de96c5f-876b-4ebb-b7a3-60900c6dab62"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "1450d7c122652115ef52febfa9e59349"
   strings:
      $s1 = "api-ms-win-core-synch-l1-2-0.dll" fullword wide
      $s2 = "!-- Enable themes for Windows common controls and dialogs (Windows XP and later) -->" fullword ascii
      $s3 = "mscordaccore.dll" fullword wide
      $s4 = "Runtime Broker.dll" fullword wide 
      $s5 = "D:\\a\\_work\\1\\s\\artifacts\\obj\\coreclr\\windows.x64.Release\\dlls\\mscordac\\mscordaccore.pdb" fullword ascii 
      $s6 = "Runtime Broker - Windows NT Mode" fullword wide 
      $s7 = "       Makes the application long-path aware. See https://docs.microsoft.com/windows/win32/fileio/maximum-file-path-limitation -" ascii 
      $s8 = "ni.dll" fullword wide 
      $s9 = "        <requestedExecutionLevel  level=\"asInvoker\" uiAccess=\"false\" />" fullword ascii 
      $s10 = "PROCESSOR_COUNT" fullword wide 
      $s11 = "Nhttp://www.microsoft.com/pkiops/crl/Microsoft%20Time-Stamp%20PCA%202010(1).crl0l" fullword ascii
      $s12 = "Phttp://www.microsoft.com/pkiops/certs/Microsoft%20Time-Stamp%20PCA%202010(1).crt0" fullword ascii 
      $s13 = "!-- Windows 7 -->" fullword ascii 
      $s14 = "!-- Windows Vista -->" fullword ascii
      $s15 = "      \"Microsoft.Extensions.DependencyInjection.VerifyOpenGenericServiceTrimmability\": true," fullword ascii
      $s16 = "!-- Windows 8 -->" fullword ascii
      $s17 = "      <dpiAware xmlns=\"http://schemas.microsoft.com/SMI/2005/WindowsSettings\">true</dpiAware>" fullword ascii
      $s18 = "!-- Windows 10 -->" fullword ascii
      $s19 = "       Makes the application long-path aware. See https://docs.microsoft.com/windows/win32/fileio/maximum-file-path-limitation -" ascii
      $s20 = "longPathAware xmlns=\"http://schemas.microsoft.com/SMI/2016/WindowsSettings\">true</longPathAware>" fullword ascii
      
   condition:
      uint16(0) == 0x5a4d and filesize < 4000KB and
      8 of them
}