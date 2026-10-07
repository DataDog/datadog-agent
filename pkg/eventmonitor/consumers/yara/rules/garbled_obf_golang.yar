rule garbled_obf_golang {
	meta:
        date = "2025-04-09"
		yarahub_reference_md5= "68b329da9893e34099c7d8ad5cb9c940"
        yarahub_uuid = "0c1c0fd8-6e61-4740-9626-bde9a82a13f0"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        // 004628aa  0fb6540448         movzx   edx, byte [rsp+rax+0x48 {var_40}]
        // 004628af  0fb6740449         movzx   esi, byte [rsp+rax+0x49 {var_40+0x1}]
        // 004628b4  89f7               mov     edi, esi
        // 004628b6  31d6               xor     esi, edx
        // 004628b8  8d3430             lea     esi, [rax+rsi]
        // 004628bb  8d76ed             lea     esi, [rsi-0x13]
        $ = { 0f b6 ?? ?? ?? 0f b6 ?? ?? ?? 89 f7 31 d6 8d ?? ?? ?? 8d ?? ?? }
    condition:
        all of them
}