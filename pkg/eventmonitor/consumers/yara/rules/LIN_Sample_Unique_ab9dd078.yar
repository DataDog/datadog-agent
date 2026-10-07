rule LIN_Sample_Unique_ab9dd078 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "ab9dd07884af5e249ab241e468da756bf3e6dcb2aea061d0e656d3812484256e.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "8a0a0aa8-93b9-443f-868c-22e0bd8edca3"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "a6ba15c7cf3243d71fe6fe3e14e4f740"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "_probe_salt=\"630471bc9f5ee4f8cb4a74532568f3692d3206c0fbbe4c89ac47de06eb69825e73dcbcb175ffe45b14f9c08b83d1ff1d\"" ascii wide
        $b = "_mem_gb=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1073741824 ))" ascii wide
    condition:
        all of them
}
