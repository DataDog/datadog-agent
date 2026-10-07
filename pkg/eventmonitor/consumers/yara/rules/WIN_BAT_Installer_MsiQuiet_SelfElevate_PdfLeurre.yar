rule WIN_BAT_Installer_MsiQuiet_SelfElevate_PdfLeurre
{
    meta:
        description = "BAT d'installation MSI silencieuse avec auto-elevation UAC et ouverture d'un PDF leurre"
        author = "Marjoriefort"
        yarahub_reference_md5 = "928ebc54bc97997f28742f45100fb4fb"
        date = "2026-09-19"
        yarahub_uuid = "c6a30268-6399-4f94-be21-42b7318f3c09"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "net session >nul 2>&1"
        $b = "-Verb RunAs"
        $c = "msiexec /i"
        $d = "/qn /norestart"
        $e = ".pdf"
    condition:
        $a and $b and $e and ($c or $d) and filesize < 10KB
}
