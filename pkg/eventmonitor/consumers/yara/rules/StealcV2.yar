rule StealcV2 {
	meta:
		author = "Still"
		component_name = "Stealc"
		date = "2025-04-26"
		description = "attempts to match the instructions found in StealcV2"
		malpedia_family = "win.stealc"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "12247ebf4653796ec00abd7c8f59b149"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "9503350b-72f6-4ae3-b22f-3646caf8e2bd"
	strings:

	/*
	0x1400231f8 FFC8                          dec eax
	0x1400231fa 410BC7                        or eax, r15d
	0x1400231fd FFC0                          inc eax
	0x1400231ff 4863F8                        movsxd rdi, eax
	0x140023202 8A0417                        mov al, byte ptr [rdi + rdx]
	0x140023205 41880410                      mov byte ptr [r8 + rdx], al
	0x140023209 44880C17                      mov byte ptr [rdi + rdx], r9b
	0x14002320d 498BD5                        mov rdx, r13
	0x140023210 49837D180F                    cmp qword ptr [r13 + 0x18], 0xf
	0x140023215 4C8B4DC8                      mov r9, qword ptr [rbp - 0x38]
	0x140023219 7604                          jbe 0x14002321f
	0x14002321b 498B5500                      mov rdx, qword ptr [r13]
	0x14002321f 430FB60408                    movzx eax, byte ptr [r8 + r9]
	 */
	/*
	0x1400231d6 4103F0                        add esi, r8d
	0x1400231d9 81E6FF000080                  and esi, 0x800000ff
	0x1400231df 7D0A                          jge 0x1400231eb
	0x1400231e1 FFCE                          dec esi
	0x1400231e3 81CE00FFFFFF                  or esi, 0xffffff00
	0x1400231e9 FFC6                          inc esi
	0x1400231eb 4863CE                        movsxd rcx, esi
	0x1400231ee 8A0419                        mov al, byte ptr [rcx + rbx]
	0x1400231f1 88041A                        mov byte ptr [rdx + rbx], al
	0x1400231f4 44880419                      mov byte ptr [rcx + rbx], r8b
	0x1400231f8 49837D180F                    cmp qword ptr [r13 + 0x18], 0xf
	0x1400231fd 0FB6041A                      movzx eax, byte ptr [rdx + rbx]
	0x140023201 498BD5                        mov rdx, r13
	0x140023204 7604                          jbe 0x14002320a
	0x140023206 498B5500                      mov rdx, qword ptr [r13]
	0x14002320a 4903C0                        add rax, r8
	0x14002320d 0FB6C0                        movzx eax, al
	 */
		$inst_rc4 = {
			8A 04 ??
			[0-8]
			44 88 [2]
			[0-8]
			49 83 7D 18 0F
			[2-8]
			76 04
			49 8B 55 00
		}
	/*
	0x14003a9e0 498BCC                        mov rcx, r12
	0x14003a9e3 FF155F810700                  call qword ptr [rip + 0x7815f]
	0x14003a9e9 B94D000000                    mov ecx, 0x4d
	0x14003a9ee 4889442450                    mov qword ptr [rsp + 0x50], rax
	0x14003a9f3 FF15C7800700                  call qword ptr [rip + 0x780c7]
	0x14003a9f9 B94C000000                    mov ecx, 0x4c
	0x14003a9fe 8BF8                          mov edi, eax
	0x14003aa00 FF15BA800700                  call qword ptr [rip + 0x780ba]
	0x14003aa06 C74424402000CC00              mov dword ptr [rsp + 0x40], 0xcc0020
	0x14003aa0e 458BCE                        mov r9d, r14d
	0x14003aa11 897C2438                      mov dword ptr [rsp + 0x38], edi
	0x14003aa15 4533C0                        xor r8d, r8d
	0x14003aa18 89442430                      mov dword ptr [rsp + 0x30], eax
	0x14003aa1c 33D2                          xor edx, edx
	0x14003aa1e 4C896C2428                    mov qword ptr [rsp + 0x28], r13
	0x14003aa23 498BCC                        mov rcx, r12
	0x14003aa26 89742420                      mov dword ptr [rsp + 0x20], esi
	0x14003aa2a FF1570800700                  call qword ptr [rip + 0x78070]
	0x14003aa30 488B7C2458                    mov rdi, qword ptr [rsp + 0x58]
	0x14003aa35 4C8D442468                    lea r8, [rsp + 0x68]
	0x14003aa3a 488BCF                        mov rcx, rdi
	0x14003aa3d 33D2                          xor edx, edx
	 */
		$inst_screenshot = {
			FF 15 ?? ?? ?? ??
			B9 4D 00 00 00
			48 89 44 24 ??
			FF 15 ?? ?? ?? ??
			B9 4C 00 00 00
			8B F8
			FF 15 ?? ?? ?? ??
			C7 44 24 ?? 20 00 CC 00
			45 8B CE
			89 7C 24 ??
			45 33 C0
			89 44 24 ??
			33 D2
			4C 89 ?? 24 ??
			49 8B ??
			89 74 24 ??
			FF 15 ?? ?? ?? ??
			[0-8]
			4C 8D 44 24 ??
			[0-4]
			33 D2
		}
	/*
	0x1400274ca C784244001000000020000        mov dword ptr [rsp + 0x140], 0x200
	0x1400274d5 488D15947F0800                lea rdx, [rip + 0x87f94]
	0x1400274dc 488D8C24B0000000              lea rcx, [rsp + 0xb0]
	0x1400274e4 E8FF310100                    call 0x14003a6e8
	0x1400274e9 BE07000000                    mov esi, 7
	0x1400274ee 48397018                      cmp qword ptr [rax + 0x18], rsi
	0x1400274f2 7603                          jbe 0x1400274f7
	0x1400274f4 488B00                        mov rax, qword ptr [rax]
	0x1400274f7 488D8C24E0000000              lea rcx, [rsp + 0xe0]
	0x1400274ff 48894C2420                    mov qword ptr [rsp + 0x20], rcx
	0x140027504 41B919010200                  mov r9d, 0x20119
	0x14002750a 4533C0                        xor r8d, r8d
	0x14002750d 488BD0                        mov rdx, rax
	0x140027510 48C7C102000080                mov rcx, 0xffffffff80000002
	 */
	/*
	0x140027349 C784242001000000020000        mov dword ptr [rsp + 0x120], 0x200
	0x140027354 488D1535A10800                lea rdx, [rip + 0x8a135]
	0x14002735b 488D8C24B0000000              lea rcx, [rsp + 0xb0]
	0x140027363 E89C2E0100                    call 0x14003a204
	0x140027368 41BD07000000                  mov r13d, 7
	0x14002736e 4C396818                      cmp qword ptr [rax + 0x18], r13
	0x140027372 7603                          jbe 0x140027377
	0x140027374 488B00                        mov rax, qword ptr [rax]
	0x140027377 488D8C24E0000000              lea rcx, [rsp + 0xe0]
	0x14002737f 48894C2420                    mov qword ptr [rsp + 0x20], rcx
	0x140027384 41B919010200                  mov r9d, 0x20119
	0x14002738a 4533C0                        xor r8d, r8d
	0x14002738d 488BD0                        mov rdx, rax
	0x140027390 48C7C102000080                mov rcx, 0xffffffff80000002
	 */
		$inst_get_cpu_info = {
			C7 84 24 ?? ?? ?? ?? 00 02 00 00
			48 8D 15 ?? ?? ?? ??
			48 8D 8C 24 ?? ?? ?? ??
			E8 ?? ?? ?? ??
			[8-12]
			76 ??
			48 8B 00
			48 8D 8C 24 ?? ?? ?? ??
			48 89 4C 24 ??
			41 B9 19 01 02 00
			45 33 C0
			48 8B D0
			48 C7 C1 02 00 00 80
		}
	/*
	0x14002a46f 4883C378                      add rbx, 0x78
	0x14002a473 483BDF                        cmp rbx, rdi
	0x14002a476 75DE                          jne 0x14002a456
	0x14002a478 807D5300                      cmp byte ptr [rbp + 0x53], 0
	0x14002a47c 7409                          je 0x14002a487
	0x14002a47e 488D4D10                      lea rcx, [rbp + 0x10]
	0x14002a482 E86DD80000                    call 0x140037cf4
	0x14002a487 4883650000                    and qword ptr [rbp], 0
	 */
		$inst_main = {
			48 83 C3 78
			48 3B DF
			75 ??
			80 7D ?? 00
			74 ??
			48 8D 4D ??
			E8
		}
	condition:
		any of them
}
