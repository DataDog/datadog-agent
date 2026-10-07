rule WIN_VBS_RAT_ScreenConnect_QuietInstall
{
    meta:
        description = "VBS installant un MSI ScreenConnect silencieux avec elevation UAC - instance screenconnect.com tierce"
        author = "Marjoriefort"
        yarahub_reference_md5 = "5ce99602fd924e27c9679809074a6ba7"
        date = "2026-09-19"

        yarahub_uuid = "8c2eb1c7-cbf4-4328-898a-61ed0d6eca38"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "ScreenConnect.ClientSetup.msi"
        $b = "screenconnect.com/Bin/"
        $c = "msiexec.exe"
        $d = "/quiet /norestart"
        $e = "runas"

    condition:
        $a and $d and ($b or $c or $e) and filesize < 10KB
}
