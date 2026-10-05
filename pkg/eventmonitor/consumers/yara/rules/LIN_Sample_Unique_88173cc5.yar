rule LIN_Sample_Unique_88173cc5 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "88173cc5073e1ab7b5d658455b3a49af638710ae49215ba3be4132136521d148.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "c500a088-98c4-4835-990f-53426967f9f7"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "a3b58c5de4d4f3e593c76b99756b6c5a"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "for p in $(grep -oE '(fileName|filePattern)=\"[^\"]+\"' /data/wuqi-admin/config/log4j2-prod.xml 2>/dev/null | cut -d'\"' -f2 | sed 's/%.*//' | sort -u); do" ascii wide
        $b = "      *messages*|*secure*|*cron*|*warn*|*info*|*error*|*app*|*log*) truncate -s 0 \"$f\" 2>/dev/null;;" ascii wide
    condition:
        all of them
}
