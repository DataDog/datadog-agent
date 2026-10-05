rule Foxveil_AMOS_CompiledAppleScript
{
    meta:
        yarahub_uuid = "820f7a63-fa3e-462e-8d35-07502368273a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9da777e98c8af3449ce27074b17379a4"
        description = "Foxveil AMOS stealer payload as compiled AppleScript (FasdUAS): UTF-16BE victim-data paths, the fixed Chrome wallet-extension ID table, and stable FasdUAS opcode runs"
        actor = "Foxveil"
        family = "AMOS"
        reference_sha256 = "7b453a77b6278f28909f3c5105c4db556a71f0a1982d897d1c8fdfb654282aa7"
        date = "2026-09-20"
        tlp = "CLEAR"

    strings:
        $magic = "FasdUAS "

        // UTF-16BE AppleScript path literals (YARA `wide` is UTF-16LE, so: hex)
        $t_safari = { 00 43 00 6F 00 6E 00 74 00 61 00 69 00 6E 00 65 00 72 00 73 00 3A 00 63
                      00 6F 00 6D 00 2E 00 61 00 70 00 70 00 6C 00 65 00 2E 00 53 00 61 00 66
                      00 61 00 72 00 69 00 3A 00 44 00 61 00 74 00 61 00 3A 00 4C 00 69 00 62
                      00 72 00 61 00 72 00 79 00 3A 00 43 00 6F 00 6F 00 6B 00 69 00 65 00 73 00 3A }
        $t_notes  = { 00 4C 00 69 00 62 00 72 00 61 00 72 00 79 00 3A 00 47 00 72 00 6F 00 75
                      00 70 00 20 00 43 00 6F 00 6E 00 74 00 61 00 69 00 6E 00 65 00 72 00 73
                      00 3A 00 67 00 72 00 6F 00 75 00 70 00 2E 00 63 00 6F 00 6D 00 2E 00 61
                      00 70 00 70 00 6C 00 65 00 2E 00 6E 00 6F 00 74 00 65 00 73 00 3A }
        // three entries of the fixed Chrome wallet-extension ID table, UTF-16BE
        $t_ext1   = { 00 61 00 62 00 61 00 6D 00 6A 00 65 00 66 00 6B 00 69 00 64 00 6E 00 67
                      00 66 00 65 00 67 00 64 00 6A 00 62 00 6D 00 66 00 66 00 64 00 6D 00 62
                      00 67 00 6A 00 67 00 70 00 61 00 6F 00 62 00 66 }
        $t_ext2   = { 00 61 00 65 00 61 00 63 00 68 00 6B 00 6E 00 6D 00 65 00 66 00 70 00 68
                      00 65 00 70 00 63 00 63 00 69 00 6F 00 6E 00 62 00 6F 00 6F 00 68 00 63
                      00 6B 00 6F 00 6E 00 6F 00 65 00 65 00 6D 00 67 }
        $t_ext3   = { 00 61 00 63 00 6D 00 61 00 63 00 6F 00 64 00 6B 00 6A 00 62 00 64 00 67
                      00 6D 00 6F 00 6C 00 65 00 65 00 62 00 6F 00 6C 00 6D 00 64 00 6A 00 6F
                      00 6E 00 69 00 6C 00 6B 00 64 00 62 00 63 00 68 }

        // FasdUAS opcode runs of three handler bodies, byte-identical in every build
        $bc1 = { 00 50 14 00 45 A0 E0 66 6C 0C 00 01 45 B2 4F 17 00 35 A2 5B E2 E3 6C 0C 00 04
                 6B 68 1B 00 03 17 00 20 A1 5B E2 E3 6C 0C 00 04 6B 68 1B 00 04 A3 A4 08 1D 00
                 07 65 0F 59 00 03 68 5B 4F 59 FF EE 5B 4F 59 FF D9 57 00 08 58 00 05 00 06 68
                 4F 66 0F 0F 0E 00 }
        $bc2 = { 00 42 14 00 37 A0 E0 2D E1 2C E2 26 45 B1 4F 2A E3 E4 E5 A1 E6 0C 00 07 6B 1F
                 45 B2 4F A1 5B E8 5C 5B 5A 6B 5C 5A A2 32 45 B3 4F A3 E0 2D E1 2C E2 26 45 B4
                 4F A4 0F 57 00 08 58 00 09 00 0A 68 4F EB 0F 0F 0E 00 }
        $bc3 = { E0 45 B1 4F 6A 45 B2 4F E1 12 00 75 6A 76 45 B3 4F 2A E2 2D 45 B4 4F 17 00 61
                 A4 5B E3 E4 6C 0C 00 05 6B 68 1B 00 05 14 00 46 A5 E6 2D E7 2C 45 B6 4F A2 A6
                 E8 2C 1E 45 B2 4F 17 00 30 A6 5B E3 E4 6C 0C 00 05 6B 68 1B 00 07 14 00 15 A7
                 E9 2C CA 25 A7 EB 2C 25 45 B8 4F A8 A3 36 47 57 00 08 58 00 0C 00 0D 68 5B 4F
                 59 FF DE 57 00 08 58 00 0C 00 0D 68 5B 4F 59 FF AD 4F A3 EE 26 45 B1 55 4F A2
                 6A 02 1D }

    condition:
        $magic at 0 and filesize > 30KB and filesize < 4MB
        and 2 of ($t_*) and 2 of ($bc*)
}
