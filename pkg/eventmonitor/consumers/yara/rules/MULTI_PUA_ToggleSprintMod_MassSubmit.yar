rule MULTI_PUA_ToggleSprintMod_MassSubmit {
    meta:
        description = "Mod Minecraft toggle-sprint-display (com.aeltumn) soumis en masse - pattern anormal + mixin-loader obfusque, PUA a confirmer"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive 569 / grappe 39 archives identiques"
        confidence  = "medium"
        note        = "Le mod aeltumn/toggle-sprint-display est legit en open-source ; la detection porte sur le PATTERN DE SOUMISSION MASSIVE + mixin obfusque. Verifier en sandbox avant blocage."
        yarahub_uuid            = "b9bb4730-a019-4ba5-955c-e784525e0a61"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "edc3f20f3b571df736560f0c82de508c"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "com/aeltumn/togglesprintdisplay/" ascii wide
        $b = "toggle-sprint-display.mixins.json" ascii wide
        $c = "github-mixin-loader-" ascii wide
        $d = "-obfuscated.jar" ascii wide
    condition:
        uint32(0) == 0x04034b50 and $a and $b and 1 of ($c, $d)
}
