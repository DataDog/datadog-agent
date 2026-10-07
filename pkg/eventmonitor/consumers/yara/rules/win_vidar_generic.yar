rule win_vidar_generic {
    meta:
        author = "dubfib"
        date = "2025-03-09"
        malpedia_family = "win.vidar"

        yarahub_uuid = "b68d13fe-d344-483f-97d8-177bc4aaec4a"
        yarahub_reference_md5 = "da8846245fb9ec49a3223f7731236c7f"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC BY 4.0"
        yarahub_reference_link = "https://github.com/dubfib/yara"

    strings:
        $asm0 = {
            6E             /* outsb */
            20 69 ??       /* and byte ptr ds:[ecx+6E], ?? */
            20 44 4F ??    /* and byte ptr ds:[edi+ecx*2+53], ?? */
            20 6D ??       /* and byte ptr ss:[ebp+6F], ?? */
            64 65 2E 24 00 /* and al, 0 */
            00 50 ??       /* add byte ptr ds:[eax+45], ?? */
            00 00          /* add byte ptr ds:[eax], al */
            4C             /* dec esp */
            01 07          /* add dword ptr ds:[edi], eax */
            00 01          /* add byte ptr ds:[ecx], al */
            FA             /* cli */
            BB 67 00 00 00 /* mov ebx, 67 */
        }

    condition:
        uint16(0) == 0x5a4d and
        any of ($asm*)
}