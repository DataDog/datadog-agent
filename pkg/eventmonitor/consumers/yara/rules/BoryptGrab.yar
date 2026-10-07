rule BoryptGrab
{
	meta:
		author = "Still"
		component_name = "BoryptGrab"
		component_name = "BlakcSeeStealer"
		date = "2026-05-22"
		description = "attempts to match strings/instructions found in BoryptGrab/BlakcSeeStealer core"
		yarahub_uuid = "ba41dbec-d318-4959-af74-5e5eaadf621f"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_reference_md5 = "fe0ae87faab66dee8facd511712c8b00"
	strings:
		$str_edge_module_1 = "CLSIDFromString failed, hr=0x%08X, clsid=%s" ascii
		$str_edge_module_2 = "IIDFromString failed, hr=0x%08X, iid=%s" ascii
		$str_edge_module_3 = "IID parsed: %s" ascii
		$str_edge_module_4 = "CoCreateInstance(Edge) failed, hr=0x%08X, err=%lu" ascii
		$str_edge_module_5 = "Calling Edge DecryptData" ascii
		$str_edge_module_6 = "Calling DecryptData" ascii
		$str_category_1 = "CallCOMDecryptData" ascii
		$str_category_2 = "GetElevationKey" ascii
		$str_category_3 = "GetBrowserMasterKey" ascii
		$str_category_4 = "KillBrowserProcesses" ascii
		$str_category_5 = "SafeCopyFile" ascii
		$str_category_6 = "ExtractDiscordTokens" ascii
		$str_category_7 = "ExtractMaxTokens" ascii
		$str_category_8 = "GetSteamUsers" ascii
		$str_category_9 = "ExtractDesktopWallets" ascii
		$str_category_10 = "ExtractFileGrabber" ascii
		$str_category_11 = "CopyTelegramData" ascii
		$str_category_12 = "CopyTdataSelective" ascii
		$str_category_13 = "CopyBrowserData" ascii
		$str_category_14 = "SendFileToServer" ascii
		$str_verbose_1 = "browserExePath NOT FOUND: %s (error=%d), falling back to chrome.exe" ascii
		$str_verbose_2 = "Reflective injection - payload size=%u bytes" ascii
		$str_verbose_3 = "Skipping (invalid AppID: " ascii
		$str_verbose_4 = "Invalid APPB prefix" ascii
		$str_verbose_5 = "v2 failed or not tried, trying IID v1 = " ascii
		$str_verbose_6 = "FINAL RESULT - COM decryption failed (empty result)" ascii
		$str_verbose_7 = "Local State NOT FOUND" ascii
		$str_verbose_8 = "SUCCESS, master key extracted: " ascii
		$str_verbose_10 = "Cannot get APPDATA path" ascii
		$str_verbose_11 = "Saved %d unique token(s) to Discord_tokens.txt" ascii
		$str_verbose_12 = "CredEnumerate failed, error=%lu" ascii
		$str_verbose_13 = "Saved %d credential(s) to credentials.txt" ascii
		$str_verbose_14 = "Cannot open loginusers.vdf" ascii
		$str_verbose_15 = "Started steam.exe, waiting for initialization..." ascii
		$str_verbose_16 = "Steam not running, starting..." ascii
		$str_verbose_18 = "Found %d user(s), processing %d wallet rules" ascii
		$str_verbose_19 = "XX_0.0.0.0_fallback_" ascii
		$str_verbose_20 = "CredEnumerate failed, error=%lu" ascii
		$str_verbose_21 = "EXCEPTION in TryAddTelegram: " ascii
		$str_verbose_22 = "EXCEPTION on entry in: " ascii
		$str_verbose_23 = "EXCEPTION in MS Store pkg" ascii
		$str_verbose_24 = "[OK] prefs.js -> Extensions/" ascii
		$str_verbose_25 = "server sent early response at %llu/%llu bytes: %.200s" ascii
        
	/*
	0x17b846e3f61 4883BC24B802000000            cmp qword ptr [rsp + 0x2b8], 0
	0x17b846e3f6a 0F843E050000                  je 0x17b846e44ae
	0x17b846e3f70 66C7442448EFBB                mov word ptr [rsp + 0x48], 0xbbef
	0x17b846e3f77 C644244ABF                    mov byte ptr [rsp + 0x4a], 0xbf
	0x17b846e3f7c 41B803000000                  mov r8d, 3
	0x17b846e3f82 488D542448                    lea rdx, [rsp + 0x48]
	0x17b846e3f87 488D8C2430020000              lea rcx, [rsp + 0x230]
	 */
		$inst_begin_collect_userinfo = {
			48 83 BC 24 ?? ?? ?? ?? 00
			0F 84 ?? ?? ?? ??
			66 C7 44 24 ?? EF BB
			C6 44 24 ?? BF
			41 B8 03 00 00 00
			48 8D 54 24 ??
			48 8D 8C 24 
		}
	/*
	0x17b846ccee3 488B9C24A8000000              mov rbx, qword ptr [rsp + 0xa8]
	0x17b846cceeb 4881FB00003000                cmp rbx, 0x300000
	0x17b846ccef2 0F8EFD000000                  jle 0x17b846ccff5
	0x17b846ccef8 488B942478010000              mov rdx, qword ptr [rsp + 0x178]
	0x17b846ccf00 4883FA0F                      cmp rdx, 0xf
	0x17b846ccf04 7635                          jbe 0x17b846ccf3b
	0x17b846ccf06 48FFC2                        inc rdx
	0x17b846ccf09 488B8C2460010000              mov rcx, qword ptr [rsp + 0x160]
	0x17b846ccf11 4881FA00100000                cmp rdx, 0x1000
	 */
		$inst_filegrabber_filter = {
			48 8B 9C 24 ?? ?? ?? ??
			48 81 FB 00 00 30 00
			0F 8E ?? ?? ?? ??
			48 8B 94 24 ?? ?? ?? ??
			48 83 FA 0F
			76 ??
			48 FF C2
			48 8B 8C 24 ?? ?? ?? ??
			48 81 FA 00 10 00 00
		}
	/*
	0x17b846a4d58 488D8D80000000                lea rcx, [rbp + 0x80]
	0x17b846a4d5f E83CA50300                    call 0x17b846df2a0
	0x17b846a4d64 81C3F4010000                  add ebx, 0x1f4
	0x17b846a4d6a 81FB30750000                  cmp ebx, 0x7530
	0x17b846a4d70 0F8C3AFFFFFF                  jl 0x17b846a4cb0
	0x17b846a4d76 458B8604090000                mov r8d, dword ptr [r14 + 0x904]
	 */
		$inst_CallCOMDecryptData = {
			48 8D 8D ?? ?? ?? ??
			E8 ?? ?? ?? ??
			81 C3 F4 01 00 00
			81 FB 30 75 00 00
			0F 8C ?? ?? ?? ??
			45 8B 86
		}
	/*
	0x17b846bc2f0 66C7442448EFBB                mov word ptr [rsp + 0x48], 0xbbef
	0x17b846bc2f7 C644244ABF                    mov byte ptr [rsp + 0x4a], 0xbf
	0x17b846bc2fc 4C896C2420                    mov qword ptr [rsp + 0x20], r13
	0x17b846bc301 4C8D4D80                      lea r9, [rbp - 0x80]
	0x17b846bc305 41B803000000                  mov r8d, 3
	 */
		$inst_bbefbf = {
			66 C7 44 24 ?? EF BB
			C6 44 24 ?? BF
			4C 89 6C 24 ??
			4C 8D 4D ??
			41 B8 03 00 00 00
		}
	condition:
		4 of ($str_edge_*) or
		8 of ($str_category_*) or
		10 of ($str_verbose_*) or
        2 of ($inst_*)
}