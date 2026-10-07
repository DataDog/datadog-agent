
rule JKornevHidden
{
	meta:
		author = "Still"
		component_name = "JKornevHidden"
		date = "2025-09-20"
		description = "attempts to match the strings found in JKornev's Hidden rootkit"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "2965ddbcd11a08a3ca159af187ef754c"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "dcad8331-f969-4830-acc5-52e9e0910760"
	strings:
		$str_1 = "Error, can't allocate memory for the config context" ascii
		$str_2 = "An internal error, looks like a stack corruption!" ascii
		$str_3 = "Error, symbolic link creation failed with code:%08x" ascii
		$str_4 = "object monitor haven't started" ascii
		$str_5 = "can't activate stealth mode" ascii
		$str_6 = "Operation is skipped for excluded process" ascii
		$str_7 = "Hidden dir is removed, id:%lld" ascii
		$str_8 = "Can't get an OS version, status:%x" ascii
		$str_9 = "Exception while disassemblying %p" ascii
		$str_10 = "this path already in a rules list" ascii
		$str_11 = "registry filter registration failed with code:%08x" ascii
		$rtti_1 = "AVCommandUnhide" ascii
		$rtti_2 = "AVCommandHide" ascii
		$rtti_3 = "AVCommandUnignore" ascii
		$rtti_4 = "AVCommandIgnore" ascii
		$rtti_5 = "AVCommandUnprotect" ascii
		$rtti_6 = "AVCommandProtect" ascii
		$rtti_7 = "AVCommandQuery" ascii
		$rtti_8 = "AVCommandState" ascii
	/*
	0x14000c273 BF50734D6E                    mov edi, 0x6e4d7350
	0x14000c278 448BC7                        mov r8d, edi
	0x14000c27b 660318                        add bx, word ptr [rax]
	0x14000c27e 0FB7D3                        movzx edx, bx
	0x14000c281 FF15C19D0000                  call qword ptr [rip + 0x9dc1]
	0x14000c287 4533E4                        xor r12d, r12d
	0x14000c28a 66895DD9                      mov word ptr [rbp - 0x27], bx
	0x14000c28e 488945DF                      mov qword ptr [rbp - 0x21], rax
	 */
		$inst_psmn_tag = {
			BF 50 73 4D 6E
			44 8B C7
			66 03 18
			0F B7 D3
			FF 15 ?? ?? ?? ??
			45 33 E4
			66 89 5D ??
			48 89 45
		}
	/*
	0x14000ba00 8B4304                        mov eax, dword ptr [rbx + 4]
	0x14000ba03 3D9D020000                    cmp eax, 0x29d
	0x14000ba08 0F849C000000                  je 0x14000baaa
	0x14000ba0e 3DA3010000                    cmp eax, 0x1a3
	0x14000ba13 0F859E000000                  jne 0x14000bab7
	0x14000ba19 488364246000                  and qword ptr [rsp + 0x60], 0
	0x14000ba1f 488D9388000000                lea rdx, [rbx + 0x88]
	0x14000ba26 4C8BC1                        mov r8, rcx
	0x14000ba29 4C8D4C2460                    lea r9, [rsp + 0x60]
	0x14000ba2e 488BCB                        mov rcx, rbx
	 */
		$inst_1 = {
			8B 43 ??
			3D 9D 02 00 00
			0F 84 ?? ?? ?? ??
			3D A3 01 00 00
			0F 85 ?? ?? ?? ??
			48 83 64 24 ?? 00
			48 8D 93 ?? ?? ?? ??
			4C 8B C1
			4C 8D 4C 24 ??
			48 8B CB
		}
	
	condition:
		4 of ($str_*) or
		5 of ($rtti_*) or 
		any of ($inst_*)
}
