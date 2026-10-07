rule SCRIPT_Sample_Unique_ac0f07e7 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "ac0f07e7b15723c4fb51a8fe33dff61f4fe07c2e897d207fdbba292f32a23080.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a944e28d-144a-4c94-9e16-8d0a9d978203"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "f490e8b59ea8a53286931a55200a1ace"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "6a22f49fcb8d565c52562eda" ascii wide
        $b = "6a22f49fcb8d565c52562eda" ascii wide
    condition:
        all of them
}
