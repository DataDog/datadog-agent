rule MAC_Sample_Unique_a981fdba {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "a981fdba66721eb21e7a3c0ab18e487247bbcf97158737ab8c80e98f4ec240dd.macho"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "22179463-b708-4634-84e9-c76675ff10aa"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6f37577cd7cf4b34fd9e7888c086692c"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "setup-1c10085c457ec835258fc3538bc32604e5b372ae" ascii wide
        $b = "/usr/lib/libSystem.B.dylib" ascii wide
    condition:
        all of them
}
