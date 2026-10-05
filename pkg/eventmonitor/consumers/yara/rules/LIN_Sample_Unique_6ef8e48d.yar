rule LIN_Sample_Unique_6ef8e48d {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "6ef8e48d652b3ddc851bbd0c239665a8fd47762a3935db2ee3e68c2a3f34bb5d.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "5e6356a0-909f-4cb9-8e3f-1243b8aa5433"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "fec34657fc23ef672d617b4bddb5e0da"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    setsid ./\"$OUT\" massload </dev/null >/dev/null 2>&1 &" ascii wide
        $b = "    i686*|i586*|i386*|x86*) FILE=manta.x86_32 ;;" ascii wide
    condition:
        all of them
}
