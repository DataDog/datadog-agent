rule WIN_VBS_Installer_MsiQuiet_PdfLeurre_UAC
{
    meta:
        description = "VBS installant un MSI en silencieux (/qn) avec elevation UAC et ouverture d'un PDF leurre - kit RAT re-package"
        author = "Marjoriefort"
        yarahub_reference_md5 = "a0a80a2ac6a1c69ffcda33c9d11e4777"
        date = "2026-09-19"
        yarahub_uuid = "b215b5ec-9c19-406a-b96b-759a7e067a00"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "/qn /norestart"
        $b = "FileProtocolHandler"
        $c = "msiexec"
        $d = "runas"
    condition:
        $a and $b and ($c or $d) and filesize < 10KB
}
