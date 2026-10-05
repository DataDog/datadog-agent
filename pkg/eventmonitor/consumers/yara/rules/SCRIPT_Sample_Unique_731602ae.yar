rule SCRIPT_Sample_Unique_731602ae {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "731602ae86c944ed94425a59bb3f0733ec16f8916ad7de23f271c2bef3606231.js"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a9e715e8-eeff-47fa-afdc-022a6dffa662"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "b0de5cead30b3c842995361c05f391d0"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "                    !YAeCXfJqqjAIaKOKigoz17179['test'](YAeCXfJqqjAIaKOKigoz51052 + YAeCXfJqqjAIaKOKigoz39012(0xfc)) || !YAeCXfJqqjAIaKOKigoz60179['test'](YAeCXfJqqjAIaKOKigoz51052 + YAeCXfJqqjAIaKOKigoz39012(0x12d)) ? YAeCXfJqqjAIaKOKigoz51052('0') : YAeCXfJqqjAIaKOKigoz36745();" ascii wide
        $b = "                var YAeCXfJqqjAIaKOKigoz68724 = new RegExp(YAeCXfJqqjAIaKOKigoz51739(0x113)), YAeCXfJqqjAIaKOKigoz38274 = new RegExp('\\x5c+\\x5c+\\x20*(?:[a-zA-Z_$][0-9a-zA-Z_$]*)', 'i'), YAeCXfJqqjAIaKOKigoz48889 = YAeCXfJqqjAIaKOKigoz67414(YAeCXfJqqjAIaKOKigoz51739(0xf7));" ascii wide
    condition:
        all of them
}
