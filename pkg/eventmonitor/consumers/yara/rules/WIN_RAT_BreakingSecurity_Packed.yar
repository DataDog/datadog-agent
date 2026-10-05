rule WIN_RAT_BreakingSecurity_Packed {
    meta:
        description = "BreakingSecurity RAT (win32) : RAT cle en main, desactive l'UAC (EnableLUA=0), persistance Run, scripts install/update.bat, capture presse-papiers et execution de commandes. Souvent packe UPX."
        author      = "Marjoriefort"
        date        = "2026-09-19"
        reference   = "Grand Scan InTheWild.0440 / miss a6ccd895 (UPX depacke 23552 -> 57344 o)"
        yarahub_reference_md5 = "4a40b6494d80287a5660b2b0fe881ddf"
        confidence  = "high"
        yarahub_uuid = "5aafd945-5bae-4383-92fd-d1663b7f37c3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "BreakingSecurity RAT" ascii wide
        $b = "Connected to C&C Interface!" ascii wide
        $c = "closeprocfromwindow" ascii wide
        $d = "\\install.bat" ascii wide
        $e = "clipboarddata" ascii wide
    condition:
        2 of ($a, $b, $c, $d, $e)
}
