rule BadIISModule
{
	meta:
		author = "Still"
		component_name = "BadIIS"
		date = "2025-09-20"
		description = "attempts to match the strings and instructions found in BadIIS"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "1ca50c2d1b82732fc6c834bbdd4e34e2"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "004c973e-8627-4740-ac2d-f97d1f1477a7"
	strings:
		$rtti_1 = "AVUploadServer" ascii
		$rtti_2 = "AVHiJackServer" ascii
		$rtti_3 = "AVRedirectServer" ascii
		$rtti_4 = "AVWebdllServer" ascii
		$rtti_5 = "AVAffLinkServer" ascii
		$rtti_6 = "AVBaseServer" ascii
		$pdb_set1_1 = "D:\\DriverSpace\\"
		$pdb_set1_2 = "\\hidden\\x64\\"
		$pdb_set1_3 = "\\Winkbj.pdb"
		$pdb_set2_1 = "\\Dongtai.pdb" ascii
		$pdb_set2_2 = "\\IIS\\IISCPP-GM\\" ascii
		$pdb_set2_3 = "D:\\IIS\\" ascii
		$str_1 = "Message: ok<br>\r\n" ascii fullword
		$str_2 = "Plugin Version: " ascii fullword
		$str_3 = "hj-plugin-iis-cpp-v" ascii
		$str_4 = "TryCleanTmp:" ascii
		$str_5 = "FAB234CD3-09434-8898D-BFFC-4E23123DF2C" ascii
		$str_6 = ",open failed" ascii fullword
		$str_7 = "DeleteFilesByPercentage" ascii
		$str_8 = "xxfailed: " ascii
		$str_9 = "PostReportNoRemoteConfig:existed!" ascii fullword
		$str_10 = "name='cmdml' placeholder='Enter CMD" ascii
	/*
	0x18003d59a 0F85740E0000                  jne 0x18003e414
	0x18003d5a0 48B96861636B31323334          movabs rcx, 0x343332316b636168
	0x18003d5aa 483908                        cmp qword ptr [rax], rcx
	0x18003d5ad 0F85610E0000                  jne 0x18003e414
	0x18003d5b3 668178083536                  cmp word ptr [rax + 8], 0x3635
	0x18003d5b9 0F85550E0000                  jne 0x18003e414
	0x18003d5bf 80780A21                      cmp byte ptr [rax + 0xa], 0x21
	0x18003d5c3 0F854B0E0000                  jne 0x18003e414
	 */
		$inst_passwd_check = {
			0F 85 ?? ?? ?? ??
			48 B9 68 61 63 6B 31 32 33 34
			48 39 08
			0F 85 ?? ?? ?? ??
			66 81 78 ?? 35 36
			0F 85 ?? ?? ?? ??
			80 78 ?? 21
			0F 85
		}
	/*
	0x18003b604 41B800300000                  mov r8d, 0x3000
	0x18003b60a FF1570FA0600                  call qword ptr [rip + 0x6fa70]
	0x18003b610 488BF8                        mov rdi, rax
	0x18003b613 48837B1810                    cmp qword ptr [rbx + 0x18], 0x10
	0x18003b618 7203                          jb 0x18003b61d
	0x18003b61a 488B1B                        mov rbx, qword ptr [rbx]
	0x18003b61d 4C8BCB                        mov r9, rbx
	0x18003b620 4C8D053D660800                lea r8, [rip + 0x8663d]
	0x18003b627 BA04010000                    mov edx, 0x104
	0x18003b62c 488BCF                        mov rcx, rdi
	0x18003b62f E8DCA1FFFF                    call 0x180035810
	0x18003b634 4C896518                      mov qword ptr [rbp + 0x18], r12
	0x18003b638 4C896528                      mov qword ptr [rbp + 0x28], r12
	0x18003b63c 4C896530                      mov qword ptr [rbp + 0x30], r12
	0x18003b640 4C896528                      mov qword ptr [rbp + 0x28], r12
	0x18003b644 48C745300F000000              mov qword ptr [rbp + 0x30], 0xf
	0x18003b64c 44886518                      mov byte ptr [rbp + 0x18], r12b
	0x18003b650 41B824000000                  mov r8d, 0x24
	 */
		$inst_alloc_cmd = {
			41 B8 00 30 00 00
			FF 15 ?? ?? ?? ??
			48 8B F8
			48 83 7B ?? 10
			72 ??
			48 8B 1B
			4C 8B CB
			4C 8D 05 ?? ?? ?? ??
			BA 04 01 00 00
			48 8B CF
			E8 ?? ?? ?? ??
			4C 89 65 ??
			4C 89 65 ??
			4C 89 65 ??
			4C 89 65 ??
			48 C7 45 ?? 0F 00 00 00
			44 88 65 ??
			41 B8 24 00 00 00
		}
	condition:
		3 of ($rtti_*) or
		2 of ($pdb_set1_*) or
		2 of ($pdb_set2_*) or
		4 of ($str_*) or
		any of ($inst_*)
}
