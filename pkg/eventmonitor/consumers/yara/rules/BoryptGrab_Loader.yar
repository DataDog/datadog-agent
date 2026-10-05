rule BoryptGrab_Loader
{
	meta:
		author = "Still"
		component_name = "BoryptGrab"
		component_name = "BlakcSeeStealer"
		date = "2026-05-22"
		description = "attempts to match strings/instructions found in BoryptGrab/BlakcSeeStealer loader"
		yarahub_uuid = "62a32bd9-9883-45cb-8c43-17069ece5b8d"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_reference_md5 = "d7f684659575575e90084fe49d9f259c"
	strings:
	/*
	0x1800089cc C645484D                      mov byte ptr [rbp + 0x48], 0x4d
	0x1800089d0 0FB64D48                      movzx ecx, byte ptr [rbp + 0x48]
	0x1800089d4 0FB6C9                        movzx ecx, cl
	0x1800089d7 660F6EC1                      movd xmm0, ecx
	0x1800089db 660F60C0                      punpcklbw xmm0, xmm0
	0x1800089df F20F70C000                    pshuflw xmm0, xmm0, 0
	0x1800089e4 F30F7E0DC4290100              movq xmm1, qword ptr [rip + 0x129c4]
	0x1800089ec 660FFCC8                      paddb xmm1, xmm0
	0x1800089f0 660FEF0D583A0100              pxor xmm1, xmmword ptr [rip + 0x13a58]
	0x1800089f8 660FD60DC00E0200              movq qword ptr [rip + 0x20ec0], xmm1
	0x180008a00 660FFC05B8290100              paddb xmm0, xmmword ptr [rip + 0x129b8]
	0x180008a08 660FEF05503A0100              pxor xmm0, xmmword ptr [rip + 0x13a50]
	0x180008a10 660F7E05B00E0200              movd dword ptr [rip + 0x20eb0], xmm0
	0x180008a18 80C17B                        add cl, 0x7b
	0x180008a1b 80F1C8                        xor cl, 0xc8
	 */
		$inst_decrypt_strings = {
			C6 45 ?? ??
			0F B6 ?? ??
			0F B6 ??
			66 0F 6E ??
			66 0F 60 ??
			[0-8]
			F3 0F 7E 0D ?? ?? ?? ??
			[0-8]
			66 0F EF 0D ?? ?? 01 00
			66 0F D6 0D ?? ?? 02 00
		}
	/*
	0x18000d4d7 BA20000000                    mov edx, 0x20
	0x18000d4dc E86F030000                    call 0x18000d850
	0x18000d4e1 4801FB                        add rbx, rdi
	0x18000d4e4 4080FE01                      cmp sil, 1
	0x18000d4e8 7526                          jne 0x18000d510
	0x18000d4ea 31F6                          xor esi, esi
	0x18000d4ec 4889F9                        mov rcx, rdi
	0x18000d4ef BA01000000                    mov edx, 1
	0x18000d4f4 4531C0                        xor r8d, r8d
	0x18000d4f7 FFD3                          call rbx
	 */
		$inst_call_shellcode = {
			BA 20 00 00 00
			E8 [4]
			48 [2]
			40 [2] 01
			75 ??
			31 F6
			48 89 F9
			BA 01 00 00 00
			45 31 C0
			FF D3
		}
	/*
	0x18000c897 7473                          je 0x18000c90c
	0x18000c899 488B05F8D20100                mov rax, qword ptr [rip + 0x1d2f8]
	0x18000c8a0 49BE0DF0ADDEBEBAFECA          movabs r14, 0xcafebabedeadf00d
	0x18000c8aa 4931C6                        xor r14, rax
	0x18000c8ad B920000000                    mov ecx, 0x20
	0x18000c8b2 E85D0F0000                    call 0x18000d814
	0x18000c8b7 4889C3                        mov rbx, rax
	 */
		$inst_cafebabedeadfood = {
			74 ??
			48 8B 05 ?? ?? ?? ??
			49 BE 0D F0 AD DE BE BA FE CA
			49 31 C6
			B9 20 00 00 00
			E8 ?? ?? ?? ??
			48 89 C3
		}
	/*
	0x180001536 3B8104000000                  cmp eax, dword ptr [rcx + 4]
	0x18000153c 0F8F21030000                  jg 0x180001863
	0x180001542 488B053F810200                mov rax, qword ptr [rip + 0x2813f]
	0x180001549 4885C0                        test rax, rax
	0x18000154c 0F84D5000000                  je 0x180001627
	0x180001552 BA00AA0D00                    mov edx, 0xdaa00
	0x180001557 31C9                          xor ecx, ecx
	 */
		$inst_VirtualAlloc_0xDAA00 = {
			3B 81 ?? ?? ?? ??
			0F 8F ?? ?? ?? ??
			48 8B 05 ?? ?? ?? ??
			48 85 C0
			0F 84 ?? ?? ?? ??
			BA 00 AA 0D 00
			31 C9
		}
	condition:
		2 of them
}
