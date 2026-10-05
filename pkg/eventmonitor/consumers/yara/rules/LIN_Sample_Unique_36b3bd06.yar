rule LIN_Sample_Unique_36b3bd06 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "36b3bd0612a12f8bf91bb25c440f3fd26051d62cdc263a1f00b99fc030c0bf41.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "08c71cd0-45ce-414a-a4a7-ece3ffa7cff5"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6cf29aa5a8f9790d4a87a6f5e566ba16"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "R=$(head -c4 /dev/urandom 2>/dev/null|od -An -tx1|tr -d ' \\n'||echo $$)" ascii wide
        $b = "[ -x \"$E\" ] && run \"[migration/0]\" \"$E\"" ascii wide
    condition:
        all of them
}
