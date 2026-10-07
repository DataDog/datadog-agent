rule WIN_Sample_Unique_69354b41 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "69354b41e10daf03d3f3af881b32d5c0fec56b1cfe96629fd4c5263413a42854.exe"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "42c7fe09-5066-4b79-bbbe-ae9fdf528fd0"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7f131d83fbcd502c65cd05e9e07af7ba"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "        <requestedExecutionLevel level=\"asInvoker\" uiAccess=\"false\"/>" ascii wide
        $b = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" ascii wide
    condition:
        all of them
}
