rule ANDROID_Sample_Unique_72544831 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "725448313dcb59422e6cb67fcc1ec11a3f058302e3007aaf0b93a9965225ab4b.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "19d0afe2-cea7-48d3-8e7b-043448a03d6c"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8ac1811e57196ae3faded4985a90e5b9"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "assets/minecraft/shaders/post-process/outline.fragPK" ascii wide
        $b = "assets/minecraft/shaders/post-process/image.fragPK" ascii wide
    condition:
        all of them
}
