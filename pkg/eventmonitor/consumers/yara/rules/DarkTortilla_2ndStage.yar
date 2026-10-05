rule DarkTortilla_2ndStage
{
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2025-01-11"
		malpedia_family = "win.darktortilla"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "A102CEE15D494DB5AE6C917374FFBFC3"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "c5f0eefa-461d-499e-8f7b-71c01091c9b7"
		description = "Matches DarkTortilla second stage loader strings/bytecode"
	strings:
		$str_1 = "_GlobalOptions" ascii
		$str_2 = "_AntiVMs" ascii
		$str_3 = "_Melt" ascii
		$str_4 = "_PreStart" ascii
		$str_5 = "has been detected on ur computer" wide
		$str_6 = "for malicious ending," wide
		$str_7 = "ping 1.1.1.1 -n 1 -w 3000 > Nul & Del" wide
		$str_8 = "F.{0}.R" wide
/* 0x0000CBF7 28CF00000A    IL_02FF: call      int32 [Microsoft.VisualBasic]Microsoft.VisualBasic.CompilerServices.Conversions::ToInteger(object)*/
/* 0x0000CBFC 6F7D000006    IL_0304: callvirt  instance void MofInagitap.Class14_Addons/AddonPackage::set_Delay(int32)*/
/* 0x0000CC01 00            IL_0309: nop*/
/* 0x0000CC02 1104          IL_030A: ldloc.s   V_4*/
/* 0x0000CC04 03            IL_030C: ldarg.1*/
/* 0x0000CC05 72581C0070    IL_030D: ldstr     "F.{0}.R"*/
/* 0x0000CC0A 09            IL_0312: ldloc.3*/
/* 0x0000CC0B 8C3A000001    IL_0313: box       [mscorlib]System.Int32*/
/* 0x0000CC10 28CD00000A    IL_0318: call      string [mscorlib]System.String::Format(string, object)*/
/* 0x0000CC15 6FCE00000A    IL_031D: callvirt  instance !1 class [mscorlib]System.Collections.Generic.Dictionary`2<object, object>::get_Item(!0)*/
/* 0x0000CC1A 28CF00000A    IL_0322: call      int32 [Microsoft.VisualBasic]Microsoft.VisualBasic.CompilerServices.Conversions::ToInteger(object)*/
/* 0x0000CC1F 6F79000006    IL_0327: callvirt  instance void MofInagitap.Class14_Addons/AddonPackage::set_ExecutionInterval(int32)*/
/* 0x0000CC24 00            IL_032C: nop*/
/* 0x0000CC25 04            IL_032D: ldarg.2*/
/* 0x0000CC26 1305          IL_032E: stloc.s   V_5*/
/* 0x0000CC28 1105          IL_0330: ldloc.s   V_5*/
/* 0x0000CC2A 2C15          IL_0332: brfalse.s IL_0349*/
/* 0x0000CC2C 1104          IL_0334: ldloc.s   V_4*/
/* 0x0000CC2E 1104          IL_0336: ldloc.s   V_4*/
/* 0x0000CC30 6F70000006    IL_0338: callvirt  instance uint8[] MofInagitap.Class14_Addons/AddonPackage::get_FileBytes()*/
/* 0x0000CC35 2880000006    IL_033D: call      uint8[] MofInagitap.Class15_Compress::GetSizeOfMethodBody(uint8[])*/
		$inst_addon_package = {
			28 [4]
			6F [4]
			00
			11 ??
			03
			72 [4]
			09
			8C [4]
			28 [4]
			6F [4]
			28 [4]
			6F [4]
			00
			04
			13 ??
			11 ??
			2C ??
			11 ??
			11 ??
			6F [4]
			28
		}
/* 0x0000C2F4 00           IL_0004: nop*/
/* 0x0000C2F5 7E2F000004   IL_0005: ldsfld    class MofInagitap.Class7_GlobalOptions MofInagitap.Class6_GetOptions::IgnoreNonSpace*/
/* 0x0000C2FA 6F09010006   IL_000A: callvirt  instance string MofInagitap.Class7_GlobalOptions::get_TempFolder()*/
/* 0x0000C2FF 03           IL_000F: ldarg.1*/
/* 0x0000C300 722A1B0070   IL_0010: ldstr     ".exe"*/
/* 0x0000C305 287700000A   IL_0015: call      string [mscorlib]System.String::Concat(string, string)*/
/* 0x0000C30A 2831000006   IL_001A: call      string MofInagitap.Class10_Path::M_compareInfo(string, string)*/
/* 0x0000C30F 0A           IL_001F: stloc.0*/
/* 0x0000C310 00           IL_0020: nop*/
/* 0x0000C311 2817000006   IL_0021: call      class OptimizeMacros Conv_Ovf_I4::SystemDrawingDesign()*/
/* 0x0000C316 6FC300000A   IL_0026: callvirt  instance class [Microsoft.VisualBasic]Microsoft.VisualBasic.MyServices.FileSystemProxy [Microsoft.VisualBasic]Microsoft.VisualBasic.Devices.ServerComputer::get_FileSystem()*/
/* 0x0000C31B 02           IL_002B: ldarg.0*/
/* 0x0000C31C 06           IL_002C: ldloc.0*/
/* 0x0000C31D 00           IL_002D: nop*/
		$inst_exe_comp = {
			7E [4]
			6F [4]
			03
			72 [4]
			28 [4]
			28 [4]
			0A
			00
			28 [4]
			6F [4]
			02
			06
		}
	condition:
		3 of ($str_*) or
		any of ($inst_*)
}
