rule SCRIPT_Sample_Unique_65b757d5 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "65b757d55be2f535183c097e46a6937aac40eb870b5ed7958282ef4fab3be920.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "ad039be9-bb6e-4493-abcb-43bf892c6c74"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "c9209bfc8a48f8bd6c6d77f893962540"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "f39c69c316da408a6b6f2aa8" ascii wide
        $b = "f39c69c316da408a6b6f2aa8" ascii wide
    condition:
        all of them
}
