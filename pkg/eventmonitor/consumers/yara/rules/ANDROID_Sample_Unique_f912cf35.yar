rule ANDROID_Sample_Unique_f912cf35 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "f912cf350ba42c1a74107a1afe1c863410872544595846d72f7f7197cd7a7d8f.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "ea3c082c-2f9a-4795-a075-7695f7c2e048"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "dcc9a3074901334d57720b16de1ba0a1"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "com/example/addon/commands/PK" ascii wide
        $b = "addon-template.mixins.jsonPK" ascii wide
    condition:
        all of them
}
