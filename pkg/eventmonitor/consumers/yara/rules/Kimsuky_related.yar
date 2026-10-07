
rule Kimsuky_related {
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2024-05-25"
		description = "attempts to match httpSpy module potentially related to Kimsuky"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "27d4ff7439694041ef86233c2b804e1f"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "6e030925-4990-4bc3-91e6-74659fad0348"
	strings:
		$str_1 = "regsvr32.exe /s %s" wide fullword
		$str_2 = "sc delete CacheDB" wide fullword
		$str_3 = "%s%sc %s >%s 2>&1" wide fullword
		$str_4 = "ctx=" ascii fullword
	/*
	0x1800aebb4 4863D0                        movsxd rdx, eax
	0x1800aebb7 48335508                      xor rdx, qword ptr [rbp + 8]
	0x1800aebbb 490FAFD2                      imul rdx, r10
	0x1800aebbf 448B4D20                      mov r9d, dword ptr [rbp + 0x20]
	0x1800aebc3 4183C101                      add r9d, 1
	 */
		$inst_fnv = {
			48 63 D0
			48 33 55 ??
			49 0F AF D2
			44 8B 4D ??
			41 83 C1 01
		}
	/*
	0x18004c35f EB9F                          jmp 0x18004c300
	0x18004c361 8B9594020000                  mov edx, dword ptr [rbp + 0x294]
	0x18004c367 339580020000                  xor edx, dword ptr [rbp + 0x280]
	0x18004c36d 885107                        mov byte ptr [rcx + 7], dl
	0x18004c370 4883C108                      add rcx, 8
	0x18004c374 488B95982B0000                mov rdx, qword ptr [rbp + 0x2b98]
	0x18004c37b 4883C214                      add rdx, 0x14
	 */
		$inst_xor = {
			EB ??
			8B 95 ?? ?? ?? ??
			33 95 ?? ?? ?? ??
			88 51 ??
			48 83 C1 08
			48 8B 95 ?? ?? ?? ??
			48 83 C2 14
		}
	/*
	0x1800010df C74424402000CC00              mov dword ptr [rsp + 0x40], 0xcc0020
	0x1800010e7 4C89E1                        mov rcx, r12
	0x1800010ea 31D2                          xor edx, edx
	0x1800010ec 4531C0                        xor r8d, r8d
	0x1800010ef 4189D9                        mov r9d, ebx
	0x1800010f2 FF15781A0E00                  call qword ptr [rip + 0xe1a78]
	0x1800010f8 0F57F6                        xorps xmm6, xmm6
	0x1800010fb 0F29B5003E0000                movaps xmmword ptr [rbp + 0x3e00], xmm6
	0x180001102 0F29B5F03D0000                movaps xmmword ptr [rbp + 0x3df0], xmm6
	0x180001109 4C8D85F03D0000                lea r8, [rbp + 0x3df0]
	0x180001110 4C89F9                        mov rcx, r15
	0x180001113 BA20000000                    mov edx, 0x20
	 */
		$inst_screenshot = {
			C7 44 24 ?? 20 00 CC 00
			4C 89 E1
			31 D2
			45 31 C0
			41 89 D9
			FF 15 ?? ?? ?? ??
			0F 57 F6
			0F 29 B5 ?? ?? ?? ??
			0F 29 B5 ?? ?? ?? ??
			4C 8D 85 ?? ?? ?? ??
			4C 89 F9
			BA 20 00 00 00
		}
	/*
	0x1800b6889 8B442468                      mov eax, dword ptr [rsp + 0x68]
	0x1800b688d 89442444                      mov dword ptr [rsp + 0x44], eax
	0x1800b6891 8B44246C                      mov eax, dword ptr [rsp + 0x6c]
	0x1800b6895 D1F8                          sar eax, 1
	0x1800b6897 89442474                      mov dword ptr [rsp + 0x74], eax
	0x1800b689b 837C247400                    cmp dword ptr [rsp + 0x74], 0
	0x1800b68a0 B9E46FDFF3                    mov ecx, 0xf3df6fe4
	0x1800b68a5 0F84B5F6FFFF                  je 0x1800b5f60
	 */
		$inst_a = {
			8B 44 24 ??
			89 44 24 ??
			8B 44 24 ??
			D1 F8
			89 44 24 ??
			83 7C 24 ?? 00
			B9 E4 6F DF F3
			0F 84 ?? ?? ?? ??
		}
	
	
	condition:
		2 of ($str_*) or 2 of ($inst_*)
}