rule ANDROID_Sample_Unique_572cfe1f {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "572cfe1f1f5d9909484ae2fafc028431d3800ace3d2f3c8aa8b6f01552689060.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "9ee26301-9beb-446c-a222-6f8e0fd09694"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7cb39a462b3fb311a9c7bcf187305725"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "bb39069d-7d18-42c1-9903-caedebdc6969.binPK" ascii wide
        $b = "META-INF/jars/fabric.mod.jsonPK" ascii wide
    condition:
        all of them
}
