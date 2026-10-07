rule GenericRedLineLike
{
	meta:
		author = "Still"
		description = "Matches RedLine-like stealer; may match its variants."
		date = "2024-04-10"
		malpedia_family = "win.redline_stealer"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "09C25A0198EB2D54A9A38D333DF61C5C"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "268ae748-bb51-40a5-921d-3dc7a32b2205"
	strings:
		/* 0x000075AD 6F3000000A   IL_0019: callvirt  instance !0 class [mscorlib]System.Collections.Generic.IEnumerator`1<string>::get_Current() */
		/* 0x000075B2 0D           IL_001E: stloc.3 */
		/* 0x000075B3 00           IL_001F: nop */
		/* 0x000075B4 07           IL_0020: ldloc.1 */
		/* 0x000075B5 2000002003   IL_0021: ldc.i4    52428800 */
		/* 0x000075BA 6A           IL_0026: conv.i8 */
		/* 0x000075BB FE04         IL_0027: clt */
		/* 0x000075BD 16           IL_0029: ldc.i4.0 */
		/* 0x000075BE FE01         IL_002A: ceq */
		/* 0x000075C0 1304         IL_002C: stloc.s   V_4 */
		/* 0x000075C2 1104         IL_002E: ldloc.s   V_4 */
		/* 0x000075C4 2C05         IL_0030: brfalse.s IL_0037 */
		/* 0x000075C6 38F5020000   IL_0032: br        IL_032C */
		$instruction_file_search = {
			6F [4]
			0D
			[0-1]
			07
			20 00 00 20 03
			6A
			FE ??
			16
			FE ??
			13 ??
			11 ??
			2C ??
			38
		}
/* 0x0000AD43 7EED020004   IL_00D3: ldsfld    class [System.Core]System.Runtime.CompilerServices.CallSite`1<class [mscorlib]System.Func`3<class [System.Core]System.Runtime.CompilerServices.CallSite, object, class IWshRuntimeLibrary.IWshShortcut>> EnvironmentChecker/'<>o__2'::'<>p__0' */
/* 0x0000AD48 110A         IL_00D8: ldloc.s   V_10 */
/* 0x0000AD4A 1109         IL_00DA: ldloc.s   V_9 */
/* 0x0000AD4C 6F56030006   IL_00DC: callvirt  instance object IWshRuntimeLibrary.IWshShell3::CreateShortcut(string) */
/* 0x0000AD51 6FA701000A   IL_00E1: callvirt  instance !2 class [mscorlib]System.Func`3<class [System.Core]System.Runtime.CompilerServices.CallSite, object, class IWshRuntimeLibrary.IWshShortcut>::Invoke(!0, !1) */
/* 0x0000AD56 130B         IL_00E6: stloc.s   V_11 */
/* 0x0000AD58 00           IL_00E8: nop */
/* 0x0000AD59 07           IL_00E9: ldloc.1 */
/* 0x0000AD5A 130C         IL_00EA: stloc.s   V_12 */
/* 0x0000AD5C 16           IL_00EC: ldc.i4.0 */
/* 0x0000AD5D 130D         IL_00ED: stloc.s   V_13 */
/* 0x0000AD5F 2B3B         IL_00EF: br.s      IL_012C */
/* 0x0000AD61 110C         IL_00F1: ldloc.s   V_12 */
/* 0x0000AD63 110D         IL_00F3: ldloc.s   V_13 */
/* 0x0000AD65 9A           IL_00F5: ldelem.ref */
/* 0x0000AD66 130E         IL_00F6: stloc.s   V_14 */
/* 0x0000AD68 00           IL_00F8: nop */
/* 0x0000AD69 110B         IL_00F9: ldloc.s   V_11 */
/* 0x0000AD6B 6F5B030006   IL_00FB: callvirt  instance string IWshRuntimeLibrary.IWshShortcut::get_TargetPath() */
/* 0x0000AD70 110E         IL_0100: ldloc.s   V_14 */
		$instruction_create_patched_lnk = {
			7E [4]
			11 ??
			11 ??
			6F [4]
			6F [4]
			13 ??
			00
			07
			13 ??
			16
			13 ??
			2B ??
			11 ??
			11 ??
			9A
			13 ??
			00
			11 ??
			6F [4]
			11
		}
/* 0x000057E6 7E32000004   IL_005E: ldsfld    string Arguments::IP */
/* 0x000057EB 7E35000004   IL_0063: ldsfld    string Arguments::Key */
/* 0x000057F0 2895000006   IL_0068: call      string StringDecrypt::Read(string, string) */
/* 0x000057F5 17           IL_006D: ldc.i4.1 */
/* 0x000057F6 8DB3000001   IL_006E: newarr    [mscorlib]System.Char */
/* 0x000057FB 25           IL_0073: dup */
/* 0x000057FC 16           IL_0074: ldc.i4.0 */
/* 0x000057FD 1F7C         IL_0075: ldc.i4.s  124 */
/* 0x000057FF 9D           IL_0077: stelem.i2 */
/* 0x00005800 6F5F00000A   IL_0078: callvirt  instance string[] [mscorlib]System.String::Split(char[]) */
/* 0x00005805 1309         IL_007D: stloc.s   V_9 */
/* 0x00005807 16           IL_007F: ldc.i4.0 */
/* 0x00005808 130A         IL_0080: stloc.s   V_10 */
/* 0x0000580A 2B22         IL_0082: br.s      IL_00A6 */
/* 0x0000580C 1109         IL_0084: ldloc.s   V_9 */
/* 0x0000580E 110A         IL_0086: ldloc.s   V_10 */
/* 0x00005810 9A           IL_0088: ldelem.ref */
/* 0x00005811 130B         IL_0089: stloc.s   V_11 */
		$instruction_read_c2_config = {
			7E [4]
			7E [4]
			28 [4]
			17
			8D [4]
			25
			16
			1F 7C
			9D
			6F [4]
			13 ??
			16
			13 ??
			2B ??
			11 ??
			11 ??
			9A
			13
		}
/* 0x0000B8B7 6F3801000A   IL_007B: callvirt  instance object [mscorlib]Microsoft.Win32.RegistryKey::GetValue(string) */
/* 0x0000B8BC 6FAA00000A   IL_0080: callvirt  instance string [mscorlib]System.Object::ToString() */
/* 0x0000B8C1 2834010006   IL_0085: call      string StringExt::StripQuotes(string) */
/* 0x0000B8C6 6FFA010006   IL_008A: callvirt  instance void Entity4::set_Id3(string) */
/* 0x0000B8CB 00           IL_008F: nop */
/* 0x0000B8CC 1105         IL_0090: ldloc.s   V_5 */
/* 0x0000B8CE 6FF9010006   IL_0092: callvirt  instance string Entity4::get_Id3() */
/* 0x0000B8D3 14           IL_0097: ldnull */
/* 0x0000B8D4 FE03         IL_0098: cgt.un */
/* 0x0000B8D6 1308         IL_009A: stloc.s   V_8 */
/* 0x0000B8D8 1108         IL_009C: ldloc.s   V_8 */
/* 0x0000B8DA 2C1B         IL_009E: brfalse.s IL_00BB */
/* 0x0000B8DC 1105         IL_00A0: ldloc.s   V_5 */
/* 0x0000B8DE 1105         IL_00A2: ldloc.s   V_5 */
/* 0x0000B8E0 6FF9010006   IL_00A4: callvirt  instance string Entity4::get_Id3() */
/* 0x0000B8E5 28E801000A   IL_00A9: call      class [System]System.Diagnostics.FileVersionInfo [System]System.Diagnostics.FileVersionInfo::GetVersionInfo(string) */
/* 0x0000B8EA 6FE901000A   IL_00AE: callvirt  instance string [System]System.Diagnostics.FileVersionInfo::get_FileVersion() */
/* 0x0000B8EF 6FF8010006   IL_00B3: callvirt  instance void Entity4::set_Id2(string) */
		$instruction_get_browsers = {
			6F [4]
			6F [4]
			28 [4]
			6F [4]
			[0-1]
			11 ??
			6F [4]
			14
			FE ??
			13 ??
			11 ??
			2C ??
			11 ??
			11 ??
			6F [4]
			28 [4]
			6F [4]
			6F
		}
	condition:
		any of them
}
