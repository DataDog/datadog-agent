rule SCRIPT_Sample_Unique_92aa0e8f {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "92aa0e8f464457467bcce7232a96de0ba9d56b4d57956c2e69d3d64adf6f976c.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "396ddc68-5180-49ea-9d74-c247ea75298b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "74fb9a5fb1b57653ff2fd3dbc6e425f0"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "cd1b48030775714b80139a5b" ascii wide
        $b = "cd1b48030775714b80139a5b" ascii wide
    condition:
        all of them
}
