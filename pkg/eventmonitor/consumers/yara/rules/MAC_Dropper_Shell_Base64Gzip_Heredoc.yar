rule MAC_Dropper_Shell_Base64Gzip_Heredoc {
    meta:
        description = "Dropper shell (zsh/bash) macOS : payload base64+gzip en heredoc PAYLOAD_<aleatoire>, syntaxe BSD base64 -D, decompression inline puis eval/echo/printf"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "Grand Scan InTheWild.0440 / 11 misses .sh a structure identique"
        yarahub_reference_md5 = "1db78cd4f71d5743c9a5719430d6ffb6"
        confidence  = "high"
        yarahub_uuid = "6fbd45a9-3d99-40c2-b686-a1df2eed2743"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $b = "base64 -D <<'PAYLOAD_" ascii
        $c = "| gunzip" ascii
        $e = /"\$[A-Za-z0-9_]{3,}"/ ascii
    condition:
        $b and $c and $e and filesize < 10KB
}
