rule MAC_PUA_AppleScript_Compiled {
    meta:
        description = "AppleScript compile (format FasdUAS) soumis a Bazaar - rare, a verifier"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "Veille glissante 2026-09-18_08 / 4ddd6f1a"
        confidence  = "medium"
        yarahub_uuid = "d6b20e6a-f3ba-4100-9f95-0c2e9a24b157"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5   = "c3ddd1095434e7a70ff9d8397ad38299"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "FasdUAS 1.101.10" ascii wide
    condition:
        $a
}
