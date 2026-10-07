/*
   YARA Rule
   Author: DeTraced Security
   Date: 2025-10-17
   Identifier: MAL_BETRAY_DROPPER_AUG25
   Reference: https://detraced.org/posts/infostealer-turned-ransomware
*/

rule MAL_BETRAY_DROPPER_AUG25_asd_installer__installer_10 {
   meta:
      description = "MAL_BETRAY_DROPPER_AUG25 - from files asd.bat, installer_.bat, installer.bat"
      author = "DeTraced Security"
      reference = "https://detraced.org/posts/infostealer-turned-ransomware"
      date = "2025-10-17"
      yarahub_author_twitter = "@DeTraced-Sec"
      yarahub_author_email = "detraced-sec@proton.me"
      yarahub_uuid = "6562379b-6e62-4d75-ba92-f19705f56642"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "a4e53bf27bcd803a74d7f5bd348faaaf"
      hash1 = "6e5f4057a81b07cd68f83fa109c4ad4ddc39615643b8a7005da6e22498ed1fd6"
      hash2 = "2b6a513badc7dfb127f8b63acd7fcadae3d498d11176706ebfe6192fd242c696"
      hash3 = "2caa4b0b35816d523995705bd901a07e0a94e952ec514d83f63159495d7f6041"
   strings:
      $x1 = "    echo UAC.ShellExecute \"cmd.exe\", \"/c %~s0 %params%\", \"\", \"runas\", 1 >> \"%temp%\\getadmin.vbs\"" fullword ascii
      $s2 = "Powershell -Command \"Set-MpPreference -ExclusionExtension exe\"" fullword ascii
      $s3 = "    echo Set UAC = CreateObject^(\"Shell.Application\"^) > \"%temp%\\getadmin.vbs\"" fullword ascii
      $s4 = "    \"%temp%\\getadmin.vbs\"" fullword ascii
      $s5 = "    del \"%temp%\\getadmin.vbs\"" fullword ascii
      $s6 = ">nul 2>&1 \"%SYSTEMROOT%\\system32\\cacls.exe\" \"%SYSTEMROOT%\\system32\\config\\system\"" fullword ascii
      $s7 = "installer.exe" fullword ascii
      $s8 = "REM --> If error flag set, we do not have admin." fullword ascii
      $s9 = "REM  --> Check for permissions" fullword ascii
      $s10 = "cd %TEMP%" fullword ascii
      $s11 = "if '%errorlevel%' NEQ '0' (" fullword ascii
      $s12 = ") else ( goto gotAdmin )" fullword ascii
      $s13 = "    pushd \"%CD%\"" fullword ascii
      $s14 = "    exit /B" fullword ascii
      $s15 = ":--------------------------------------" fullword ascii
      $s16 = ":-------------------------------------" fullword ascii
      $s17 = "    CD /D \"%~dp0\"" fullword ascii
   condition:
      ( uint16(0) == 0xfeff and filesize < 2KB and ( 1 of ($x*) and 4 of them )
      ) or ( all of them )
}