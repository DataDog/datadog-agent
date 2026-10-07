rule LIN_Sample_Unique_658795ae {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "658795aec64a3fc84cdc6ccd8af9d2013c52882e9015b659a0fe7139cb8cd3ca.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "2b7b17ac-308f-4366-956e-8cbdadf0b3fc"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "9f17840cb42a5352910ab0fe66f7ed9c"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "nhg5o5=$(printf '\\150\\164\\164\\160\\163://\\146\\157\\162\\147\\145\\167\\151\\154\\154\\157\\167.\\143\\157\\155/2\\153\\161\\131\\122\\1150\\104\\103\\162\\156\\171\\112\\147\\157\\1234\\147\\126\\114\\154\\137\\106\\110\\112\\122\\122\\144\\124\\125\\150\\107\\103\\142\\152\\171\\165\\131\\167\\160\\1326\\143/\\153\\151\\163/\\165\\160\\144\\141\\164\\145')" ascii wide
        $b = "bzbef9=$(printf '/\\164\\155\\160/.\\164\\155\\143\\170\\160\\153\\16289')" ascii wide
    condition:
        all of them
}
