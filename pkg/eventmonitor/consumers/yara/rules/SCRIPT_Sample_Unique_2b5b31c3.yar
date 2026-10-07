rule SCRIPT_Sample_Unique_2b5b31c3 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "2b5b31c3a64b212475dbd732fa4bf714ddb4c289963d8f128e81db1aa9a677e2.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "9b9a0e1f-da30-4a29-bf82-30eed69819b7"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "c6350497589e04dc5dacedcb03937ca1"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "# === Cursor IDE: chunk scan using .IndexOf (fast in .NET) ===" ascii wide
        $b = "$fs=[System.IO.File]::Open($v,'Open','Read','ReadWrite')" ascii wide
    condition:
        all of them
}
