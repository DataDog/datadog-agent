rule win_lumma_generic {
    meta:
        author = "dubfib"
        date = "2025-02-15"
        malpedia_family = "win.lumma"

        yarahub_uuid = "c2295010-2674-4e4d-9540-6db50556ca0e"
        yarahub_reference_md5 = "5cd741616410effcd71b9c0286292ab9"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC BY 4.0"
        yarahub_reference_link = "https://github.com/dubfib/yara"

    strings:
        $asm0 = {
            55 /* push edp */
            53 /* push ebx */
            57 /* push edi */
            56 /* push esi */
            81 EC ?? 02 00 00 /* sub esp, ??? */
            E8 ?? ?? 03 00  /* call with offset +000003?? */
        }
        
    condition:
        uint16(0) == 0x5a4d and
        any of ($asm*)
}