rule Stealer_Stealc
{
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2024-10-04"
		description = "attempts to match instructions/strings found in Stealc"
		malpedia_family = "win.stealc"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "2E84B07EA9D624E7D3DBE3F95C6DD8BA"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "858820c6-ce4e-41c8-9a5b-9098dd2a4746"
	strings:
		$str_1 = "-nop -c \"iex(New-Object Net.WebClient).DownloadString('" ascii
		$str_2 = "SELECT service, encrypted_token FROM token_service" ascii
		$str_3 = "browser: FileZilla\n" ascii
		$str_4 = "ChromeFuckNewCookies" ascii
		$str_5 = "/c timeout /t 10 & del /f /q \"" ascii
	/*
	0x419750 55                            push ebp
	0x419751 8BEC                          mov ebp, esp
	0x419753 51                            push ecx
	0x419754 C745FC00000000                mov dword ptr [ebp - 4], 0
	0x41975b 64A130000000                  mov eax, dword ptr fs:[30h]
	0x419761 8B400C                        mov eax, dword ptr [eax + 0ch]
	0x419764 8B400C                        mov eax, dword ptr [eax + 0ch]
	0x419767 8B00                          mov eax, dword ptr [eax]
	0x419769 8B00                          mov eax, dword ptr [eax]
	0x41976b 8B4018                        mov eax, dword ptr [eax + 18h]
	0x41976e 8945FC                        mov dword ptr [ebp - 4], eax
	0x419771 8B45FC                        mov eax, dword ptr [ebp - 4]
	0x419774 8BE5                          mov esp, ebp
	0x419776 5D                            pop ebp
	0x419777 C3                            ret 
	 */
		$inst_low_match_peb = {
			55
			8B EC
			51
			C7 45 ?? 00 00 00 00
			64 A1 ?? ?? ?? ??
			8B 40 ??
			8B 40 ??
			8B 00
			8B 00
			8B 40 ??
			89 45 ??
			8B 45 ??
			8B E5
			5D
			C3
		}
	/*
	0x4046e6 034DF4                        add ecx, dword ptr [ebp - 0ch]
	0x4046e9 0FBE19                        movsx ebx, byte ptr [ecx]
	0x4046ec 8B550C                        mov edx, dword ptr [ebp + 0ch]
	0x4046ef 52                            push edx
	0x4046f0 FF15E0E04100                  call dword ptr [41e0e0h]
	0x4046f6 83C404                        add esp, 4
	0x4046f9 8BC8                          mov ecx, eax
	0x4046fb 8B45F4                        mov eax, dword ptr [ebp - 0ch]
	0x4046fe 33D2                          xor edx, edx
	 */
		$inst_low_match_str_decode = {
			03 4D ??
			0F BE 19
			8B 55 ??
			52
			FF 15 ?? ?? ?? ??
			83 C4 04
			8B C8
			8B 45 ??
			33 D2
		}
	condition:
		3 of ($str_*) or all of ($inst_low_match_*)
}
