rule SCRIPT_Sample_Unique_8f2c29c1 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "8f2c29c1cb874bf0bef382b6b3c5da93c5cca4622bc59a758cc6dc24f9821e7a.bat"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "274395d7-daa2-4f98-a736-2c2b775218cf"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7534c4d2cce04d7cab002e4896b55b21"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = ":: Kill any python that spawned explorer.exe (cleanup of a prior beacon)" ascii wide
        $b = "        for %%F in (*.py) do start \"\" /b \"%%~D\\python.exe\" \"%%~F\"" ascii wide
    condition:
        all of them
}
