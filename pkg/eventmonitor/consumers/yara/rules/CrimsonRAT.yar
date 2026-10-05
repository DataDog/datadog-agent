rule CrimsonRAT
{
	meta:
		author = "Still"
		description = "Matches CrimsonRAT"
		date = "2024-04-20"
		malpedia_family = "win.crimson"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "22ce9042f6f78202c6c346cef1b6e532"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "22ddb8dc-69d6-4613-abde-b5ee241593bd"
	strings:
/* 0x00002ADA 1B            IL_0002: ldc.i4.5*/
/* 0x00002ADB 8D50000001    IL_0003: newarr    [mscorlib]System.Byte*/
/* 0x00002AE0 0A            IL_0008: stloc.0*/
/* 0x00002AE1 02            IL_0009: ldarg.0*/
/* 0x00002AE2 02            IL_000A: ldarg.0*/
/* 0x00002AE3 7B33000004    IL_000B: ldfld     class [System]System.Net.Sockets.NetworkStream vteijam_hdgtra.MIETDIM::newWam*/
/* 0x00002AE8 06            IL_0010: ldloc.0*/
/* 0x00002AE9 16            IL_0011: ldc.i4.0*/
/* 0x00002AEA 1B            IL_0012: ldc.i4.5*/
/* 0x00002AEB 6FBD00000A    IL_0013: callvirt  instance int32 [mscorlib]System.IO.Stream::Read(uint8[], int32, int32)*/
/* 0x00002AF0 7D37000004    IL_0018: stfld     int32 vteijam_hdgtra.MIETDIM::byteAdesr*/
/* 0x00002AF5 06            IL_001D: ldloc.0*/
/* 0x00002AF6 16            IL_001E: ldc.i4.0*/
/* 0x00002AF7 28BE00000A    IL_001F: call      int32 [mscorlib]System.BitConverter::ToInt32(uint8[], int32)*/
/* 0x00002AFC 0B            IL_0024: stloc.1*/
/* 0x00002AFD 07            IL_0025: ldloc.1*/
/* 0x00002AFE 8D50000001    IL_0026: newarr    [mscorlib]System.Byte*/
/* 0x00002B03 0C            IL_002B: stloc.2*/
/* 0x00002B04 16            IL_002C: ldc.i4.0*/
/* 0x00002B05 0D            IL_002D: stloc.3*/
/* 0x00002B06 07            IL_002E: ldloc.1*/
/* 0x00002B07 1304          IL_002F: stloc.s   V_4*/
/* 0x00002B09 2B42          IL_0031: br.s      IL_0075*/
		$inst_process_command_code = {
			1B
			8D [4]
			0A
			02
			02
			7B [4]
			06
			16
			1B
			6F [4]
			7D [4]
			06
			16
			28 [4]
			0B
			07
			8D [4]
			0C
			16
			0D
			07
			13 ??
			2B
		}

/* 0x00002546 17           IL_007E: ldc.i4.1*/
/* 0x00002547 8D36000001   IL_007F: newarr    [mscorlib]System.Char*/
/* 0x0000254C 1305         IL_0084: stloc.s   V_5*/
/* 0x0000254E 1105         IL_0086: ldloc.s   V_5*/
/* 0x00002550 16           IL_0088: ldc.i4.0*/
/* 0x00002551 1F7C         IL_0089: ldc.i4.s  124*/
/* 0x00002553 9D           IL_008B: stelem.i2*/
/* 0x00002554 1105         IL_008C: ldloc.s   V_5*/
/* 0x00002556 6F2900000A   IL_008E: callvirt  instance string[] [mscorlib]System.String::Split(char[])*/
/* 0x0000255B 16           IL_0093: ldc.i4.0*/
/* 0x0000255C 9A           IL_0094: ldelem.ref*/
/* 0x0000255D 282D00000A   IL_0095: call      string [mscorlib]System.String::Concat(string, string, string)*/
/* 0x00002562 0C           IL_009A: stloc.2*/
/* 0x00002563 08           IL_009B: ldloc.2*/
/* 0x00002564 02           IL_009C: ldarg.0*/
/* 0x00002565 07           IL_009D: ldloc.1*/
/* 0x00002566 6FA400000A   IL_009E: callvirt  instance int64 [mscorlib]System.IO.FileInfo::get_Length()*/
/* 0x0000256B 2831000006   IL_00A3: call      instance string jevisvmanr.MIDEFORM::geyEwize(int64)*/
/* 0x00002570 7215010070   IL_00A8: ldstr     ">"*/
/* 0x00002575 282D00000A   IL_00AD: call      string [mscorlib]System.String::Concat(string, string, string)*/
/* 0x0000257A 0C           IL_00B2: stloc.2*/
		$inst_get_file_info = {
			17
			8D [4]
			13 ??
			11 ??
			16
			1F 7C
			9D
			11 ??
			6F [4]
			16
			9A
			28 [4]
			0C
			08
			02
			07
			6F [4]
			28 [4]
			72 [4]
			28 [4]
			0C
		}

/* 0x00002E4B 02            IL_000B: ldarg.0*/
/* 0x00002E4C 02            IL_000C: ldarg.0*/
/* 0x00002E4D 7B2A000004    IL_000D: ldfld     class [System]System.Net.Sockets.NetworkStream jevisvmanr.MIDEFORM::newidarm*/
/* 0x00002E52 07            IL_0012: ldloc.1*/
/* 0x00002E53 16            IL_0013: ldc.i4.0*/
/* 0x00002E54 1B            IL_0014: ldc.i4.5*/
/* 0x00002E55 6FBD00000A    IL_0015: callvirt  instance int32 [mscorlib]System.IO.Stream::Read(uint8[], int32, int32)*/
/* 0x00002E5A 7D23000004    IL_001A: stfld     int32 jevisvmanr.MIDEFORM::bytevtesa*/
/* 0x00002E5F 07            IL_001F: ldloc.1*/
/* 0x00002E60 16            IL_0020: ldc.i4.0*/
/* 0x00002E61 28BE00000A    IL_0021: call      int32 [mscorlib]System.BitConverter::ToInt32(uint8[], int32)*/
/* 0x00002E66 0C            IL_0026: stloc.2*/
/* 0x00002E67 08            IL_0027: ldloc.2*/
/* 0x00002E68 8D47000001    IL_0028: newarr    [mscorlib]System.Byte*/
/* 0x00002E6D 0D            IL_002D: stloc.3*/
/* 0x00002E6E 08            IL_002E: ldloc.2*/
/* 0x00002E6F 1304          IL_002F: stloc.s   V_4*/
/* 0x00002E71 2B42          IL_0031: br.s      IL_0075*/
		$inst_process_data = {
			02
			02
			7B [4]
			07
			16
			1B
			6F [4]
			7D [4]
			07
			16
			28 [4]
			0C
			08
			8D [4]
			0D
			08
			13 ??
			2B 
		}
	condition:
		any of them
}
