rule Heuristics_ChromeCookieMonster {
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2024-10-04"
		description = "attempts to match strings related to Chromium's CookieMonster; typically used in Chromium secrets scanning by stealers; heuristics rule - may match false positives"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "2EC23E83E2F63AB27C25741B1F4D7F49"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "06a64dd1-d590-442a-997f-000293fabc65"
	strings:
		$str_1 = "network.mojom.NetworkService" ascii wide 
		$str_2 = "chrome.dll" ascii wide
		$str_byte_pattern_1 = {56574883EC384889CE488B05AA}
		$str_byte_pattern_1_1 = "56574883EC384889CE488B05AA"
		$str_byte_pattern_1_2 = {
			C6 [2-6] 56
			C6 [2-6] 57
			C6 [2-6] 48
			C6 [2-6] 83
			C6 [2-6] EC
			C6 [2-6] 28
			C6 [2-6] 89
			C6 [2-6] D7
			C6 [2-6] 48
		}
		$str_byte_pattern_2 = {01 00 00 4C 8D 44 24 28 49 89 10 48}
		$str_byte_pattern_2_1 = "0100004C8D44242849891048"
		$str_byte_pattern_2_2 = {
			C6 [2-6] 01
			C6 [2-6] 00
			C6 [2-6] 00
			C6 [2-6] 4c
			C6 [2-6] 8d
			C6 [2-6] 44
			C6 [2-6] 24
			C6 [2-6] 28
			C6 [2-6] 49
			C6 [2-6] 89
		}
	/*
	0x140001862 33DB                          xor ebx, ebx
	0x140001864 488BF0                        mov rsi, rax
	0x140001867 4885C0                        test rax, rax
	0x14000186a 0F84B6000000                  je 140001926h
	0x140001870 4C8D4C2448                    lea r9, [rsp + 48h]
	0x140001875 C744242003000000              mov dword ptr [rsp + 20h], 3
	0x14000187d 448BC7                        mov r8d, edi
	0x140001880 488BD0                        mov rdx, rax
	0x140001883 488BCD                        mov rcx, rbp
	 */
		$inst_K32EnumProcessModulesEx = {
			33 DB
			48 8B F0
			48 85 C0
			0F 84 ?? ?? ?? ??
			4C 8D 4C 24 ??
			C7 44 24 ?? 03 00 00 00
			44 8B C7
			48 8B D0
			48 8B CD
		}
	/*
	0x140001af0 33C9                          xor ecx, ecx
	0x140001af2 85DB                          test ebx, ebx
	0x140001af4 7507                          jne 140001afdh
	0x140001af6 3D2B010000                    cmp eax, 12bh
	0x140001afb 7576                          jne 140001b73h
	0x140001afd 4C8B442438                    mov r8, qword ptr [rsp + 38h]
	0x140001b02 4D8BCE                        mov r9, r14
	0x140001b05 4C2BC5                        sub r8, rbp
	0x140001b08 4C2BCE                        sub r9, rsi
	0x140001b0b 4885ED                        test rbp, rbp
	 */
		$inst_scan_memory = {
			33 C9
			85 DB
			75 ??
			3D 2B 01 00 00
			75 ??
			4C 8B 44 24 ??
			4D 8B CE
			4C 2B C5
			4C 2B CE
			48 85 ED
		}
	condition:
		4 of ($str_*) or all of ($inst_*)
}
