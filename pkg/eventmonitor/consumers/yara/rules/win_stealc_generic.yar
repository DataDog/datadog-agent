rule win_stealc_generic {
    meta:
        author = "dubfib"
        date = "2025-03-08"
        malpedia_family = "win.stealc"

        yarahub_uuid = "8b4c19dc-7dce-4327-b2b4-feefcac2c3ca"
        yarahub_reference_md5 = "f0ad59c5e3eb8da5cbbf9c731371941c"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC BY 4.0"
        yarahub_reference_link = "https://github.com/dubfib/yara"

    strings:
        $asm0 = {
            64 EA D8 87 0B 9C 73 87 /* jmp far 8773:9C0B87D8 */
            7C ??                   /* jl [??] */
            D8 87 0B 9C 46 87       /* fadd dword ptr ds:[edi-78B963F5] */
            69 EA D8 87 0B 9C       /* imul ebp, edx, 9C0B87D8 */
            72 ??                   /* jb [??] */
            5E                      /* pop esi */
            EA D8 87 6D 92 5B 87    /* jmp far 875B:926D87D8 */
            67 EA D8 87 6D 92 4B 87 /* jmp far 874B:926D87D8 */
            62                      /* ??? */
            EA D8 87 E4 93 D9 86    /* jmp far 86D9:93E487D8 */
            67 EA D8 87 64 EA D9 87 /* jmp far 87D9:EA6487D8 */
            09 EA                   /* or edx, ebp */
            D8 87 0B 9C 77 87       /* fadd dword ptr ds:[edi-788863F5] */
            77 ??                   /* ja [??] */
            D8 87 0B 9C 45 87       /* fadd dword ptr ds:[edi-78BA63F5] */
            65 EA D8 87 52 69 63 68 /* jmp far 6863:695287D8 */
            64 EA D8 87 00 00 00 00 /* jmp far 0:87D8 */
        }
        
    condition:
        uint16(0) == 0x5a4d and
        any of ($asm*)
}