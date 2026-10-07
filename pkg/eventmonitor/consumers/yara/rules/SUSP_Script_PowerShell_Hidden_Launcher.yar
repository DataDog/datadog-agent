rule SUSP_Script_PowerShell_Hidden_Launcher {
    meta:
        description = "Script (JS/VBS) lancant PowerShell en mode furtif - dropper frais MalwareBazaar"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / cluster misses web"
        confidence  = "medium"
        yarahub_uuid            = "20c07c44-ff53-4fd5-b37a-d14a1c8eeb88"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "e847743b66dddcfc3afe248a42a71928"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "WindowStyle" ascii wide
        $b = "-ExecutionPolicy Bypass" ascii wide
        $c = "NoProfile" ascii wide
        $d = "WScript.Shell" ascii wide
        $e = "ActiveXObject" ascii wide
        $f = "powershell" ascii wide nocase
    condition:
        $f and 1 of ($a, $b, $c) and 1 of ($d, $e)
}
