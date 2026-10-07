rule Vile_Ransomware
{
    meta:
        author                      = "Arrbat"
        date                        = "2026-05-18"
        family                      = "win.vile"
        description                 = "Detects Vile Ransomware, based on family-specific markers and ransom-note strings."
        yarahub_author_github       = "Arrbat"
        yarahub_reference_md5       = "2d36014c028d86c3580a6ceb8237e601"
        yarahub_uuid                = "e35736df-3ab2-4efc-a36e-115cb497a75a"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"

    strings:
        /* Family-specific markers */
        $vile_ext      = ".vile" ascii wide nocase
        $vile_decrypt  = "vile_Decryptor" ascii wide nocase
        $vile_readme   = "VILE_README" ascii wide nocase
        $vile_wall     = "vilewall.png" ascii wide nocase
        $vile_icon     = "vileicon.ico" ascii wide nocase
        $vile_dbg      = "C:/Users/urija/Downloads/vile.go" ascii wide nocase
        $dropbox_path  = "dropbox.com/scl/fi/" ascii wide nocase

        /* Ransom note strings */
        $note_1 = "ENCRYPTION SUMMARY" ascii wide nocase
        $note_2 = "WHAT HAPPENED?" ascii wide nocase
        $note_3 = "CONTACT" ascii wide nocase
        $note_4 = "DEADLINE" ascii wide nocase
        $note_5 = "HOW TO RECOVER YOUR FILES" ascii wide nocase
        $note_6 = "PAYMENT" ascii wide nocase
        $note_7 = "WARNINGS" ascii wide nocase
        $note_8 = "YOUR FILES HAVE BEEN ENCRYPTED" ascii wide nocase

    condition:
        uint16(0) == 0x5A4D and
        2 of ($vile_ext, $vile_decrypt, $vile_readme, $vile_wall, $vile_icon, $vile_dbg, $dropbox_path) and
        4 of ($note_*) 
}
