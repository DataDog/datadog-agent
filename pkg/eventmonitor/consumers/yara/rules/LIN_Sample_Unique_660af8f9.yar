rule LIN_Sample_Unique_660af8f9 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "660af8f9b9259dc87293c3cacbdd51aec38326d10c187d6c57377e6d41b62e83.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "e1660214-8935-4535-aec2-01ad69754b78"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "b7574e9939fa61aaca4b090dee4772f8"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "case \"$M\" in armv7*) if dl \"$B/arm7\"; then chmod 777 q; nohup ./q cgi >/dev/null 2>&1 & exit 0; fi ;; esac" ascii wide
        $b = "# Quantum3 arch-detecting loader (executed on ASUS routers after AiCloud RCE)." ascii wide
    condition:
        all of them
}
