rule LIN_Sample_Unique_c0082a64 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "c0082a64f364cb6dc9be6a9f8ff2f6313a0377e3a87d864d9e5dbe40003acb59.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "2f59ecb7-27e4-4625-8ec9-b5b56f258bfa"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6c7df75401aa5e0487d004d93f62c6c9"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    local b=$(head /dev/urandom | tr -dc a-z0-9 | head -c 6)" ascii wide
        $b = "            ./\"$b\" bot.payload >/dev/null 2>&1 &" ascii wide
    condition:
        all of them
}
