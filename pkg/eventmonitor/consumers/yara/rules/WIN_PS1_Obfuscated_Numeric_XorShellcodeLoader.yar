rule WIN_PS1_Obfuscated_Numeric_XorShellcodeLoader {
    meta:
        description = "Loader PowerShell obfusque par expressions arithmetiques : tableau [Byte[]] chiffre, boucle XOR maison (for imbriques -bxor), invocation finale par variables. Famille a prefixe commun de 4 Mo."
        author      = "Marjoriefort"
        date        = "2026-09-19"
        reference   = "Grand Scan InTheWild.0440 / misses 3f8781f8 + b71b7796 (prefixe identique 4229601 octets)"
        yarahub_reference_md5 = "716f3a0bdaa261cffb68cc455e9ccbab"
        confidence  = "high"
        yarahub_uuid = "cd94921d-7cb4-430a-8ad6-782e3fde130d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "$GMqWAaVJ = 402" ascii
        $b = "$LzOxT = 167" ascii
        $c = "-bxor$qxvCMUfL[$j]" ascii
        $d = "($AkFLPWRlV)()" ascii
    condition:
        2 of ($a, $b, $c, $d) and filesize > 1MB
    }
