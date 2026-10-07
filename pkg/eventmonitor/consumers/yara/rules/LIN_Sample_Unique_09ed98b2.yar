rule LIN_Sample_Unique_09ed98b2 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "09ed98b2c1ed84a1a2dfa985ded7018e0c16b569354714916c3f2552195fabba.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "cd9ddb67-86b2-49bc-84e3-79a829ffb5ca"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "3b118c2f63dd7770eb0a87aebef390f6"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "n=\"TLYB TRPN ZBCK DSXI KJBK TROT KVKV NSXD NMLF ZBZP XUFM MKCP MFOA XOGS HQMY MGTE\"" ascii wide
        $b = "    chmod +x $a" ascii wide
    condition:
        all of them
}
