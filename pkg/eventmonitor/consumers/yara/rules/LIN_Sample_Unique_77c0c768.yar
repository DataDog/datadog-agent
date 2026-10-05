rule LIN_Sample_Unique_77c0c768 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "77c0c768861ba1896fa9c54ca6310c4797ecf0b0c2f14338f390694b542aa211.elf"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "ce950c43-a6f6-40ad-9dbf-f8c922d44910"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "3f261b235f15b389a6d8f2fc89706195"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "GCC: (GNU) 4.4.7 20120313 (Red Hat 4.4.7-23)" ascii wide
        $b = ".note.gnu.build-id" ascii wide
    condition:
        all of them
}
