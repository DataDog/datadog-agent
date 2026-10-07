rule ANDROID_Sample_Unique_4c768b6f {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "4c768b6f542f1a9ec13959206f50b3d416f164803ea81a0e1f6e4dc43d568c3a.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "1ba444ff-25ce-4677-99ca-7232e4eb94af"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6f89a649296d1e0c9aa24f3ffe7054ea"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/xenon/textures/gui/sprites/hud/heart/absorption_half.pngPK" ascii wide
        $b = "assets/xenon/textures/gui/sprites/hud/heart/absorption_half.png" ascii wide
    condition:
        all of them
}
