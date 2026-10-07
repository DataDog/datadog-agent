rule DarkTortilla_1stStage
{
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2025-01-11"
		malpedia_family = "win.darktortilla"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "CE23E784C492814093F9056ABD00080F"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "4e908ca9-155e-4d51-8c45-07987a90b2ac"
		description = "Matches DarkTortilla first stage loader strings/bytecode"
	strings:
		$str_guid = "8a7cfae3-df2a-4003-acbd-c3480d51e4ee" ascii fullword
		$str_key = "Cc7x2YJs4w0S5XoZy9m8K" ascii fullword
		/* 0x000028B5 06           IL_00A5: ldloc.0*/
		/* 0x000028B6 17           IL_00A6: ldc.i4.1*/
		/* 0x000028B7 DA           IL_00A7: sub.ovf*/
		/* 0x000028B8 7231020070   IL_00A8: ldstr     "839370"*/
		/* 0x000028BD 2866000006   IL_00AD: call      uint8[] Ki74W.q0X5D::d2J7Q(string)*/
		/* 0x000028C2 2848000006   IL_00B2: call      uint8[] c6G3S.y1LGw::Nf57L(uint8[])*/
		/* 0x000028C7 A2           IL_00B7: stelem.ref*/
		/* 0x000028C8 1A           IL_00B8: ldc.i4.4*/
		/* 0x000028C9 1307         IL_00B9: stloc.s   V_7*/
		/* 0x000028CB 3843FFFFFF   IL_00BB: br        IL_0003*/
		/* 0x000028D0 07           IL_00C0: ldloc.1*/
		/* 0x000028D1 740E00001B   IL_00C1: castclass object[]*/
		/* 0x000028D6 06           IL_00C6: ldloc.0*/
		/* 0x000028D7 2890000006   IL_00C7: call      void Go58W.b4LYd::Mj2b3(object[], int32)*/
		/* 0x000028DC DE0F         IL_00CC: leave.s   IL_00DD*/
		$inst_load = {
			06
			17
			DA
			72 [4]
			28 [4]
			28 [4]
			A2
			1A
			13 ??
			38 [4]
			07
			74 [4]
			06
			28 [4]
			DE
		}

		/* 0x0000252F 8D03000001  IL_008F: newarr    [mscorlib]System.Object  */
		/* 0x00002534 A2          IL_0094: stelem.ref  */
		/* 0x00002535 14          IL_0095: ldnull  */
		/* 0x00002536 14          IL_0096: ldnull  */
		/* 0x00002537 14          IL_0097: ldnull  */
		/* 0x00002538 17          IL_0098: ldc.i4.1  */
		/* 0x00002539 286B00000A  IL_0099: call      object [Microsoft.VisualBasic]Microsoft.VisualBasic.CompilerServices.NewLateBinding::LateCall(object, class [mscorlib]System.Type, string, object[], string[], class [mscorlib]System.Type[], bool[], bool)  */
		/* 0x0000253E 26          IL_009E: pop  */
		/* 0x0000253F 7E0C000004  IL_009F: ldsfld    char[] Jr37M.o9G3E::F  */
		/* 0x00002544 20AC000000  IL_00A4: ldc.i4    172  */
		/* 0x00002549 7E0C000004  IL_00A9: ldsfld    char[] Jr37M.o9G3E::F  */
		/* 0x0000254E 20AC000000  IL_00AE: ldc.i4    172  */
		/* 0x00002553 93          IL_00B3: ldelem.u2  */
		/* 0x00002554 7E4F000004  IL_00B4: ldsfld    char[] Go58W.b4LYd::o  */
		/* 0x00002559 20C5000000  IL_00B9: ldc.i4    197  */
		$inst_reflection = {
			8D [4]
			A2
			14
			14
			14
			17
			28 [4]
			26
			7E [4]
			20 [4]
			7E [4]
			20 [4]
			93
			7E [4]
			20
		}
		/* 0x00001974 1304         IL_00E4: stloc.s   V_4*/
		/* 0x00001976 1104         IL_00E6: ldloc.s   V_4*/
		/* 0x00001978 7456000001   IL_00E8: castclass [mscorlib]System.Security.Cryptography.Aes*/
		/* 0x0000197D 14           IL_00ED: ldnull*/
		/* 0x0000197E FE03         IL_00EE: cgt.un*/
		/* 0x00001980 130B         IL_00F0: stloc.s   V_11*/
		/* 0x00001982 110B         IL_00F2: ldloc.s   V_11*/
		/* 0x00001984 2C05         IL_00F4: brfalse.s IL_00FB*/
		/* 0x00001986 17           IL_00F6: ldc.i4.1*/
		/* 0x00001987 1310         IL_00F7: stloc.s   V_16*/
		/* 0x00001989 2BBD         IL_00F9: br.s      IL_00B8*/
		/* 0x0000198B 19           IL_00FB: ldc.i4.3*/
		/* 0x0000198C 2BF9         IL_00FC: br.s      IL_00F7*/
		/* 0x0000198E 1104         IL_00FE: ldloc.s   V_4*/
		/* 0x00001990 7456000001   IL_0100: castclass [mscorlib]System.Security.Cryptography.Aes*/
		/* 0x00001995 08           IL_0105: ldloc.2*/
		/* 0x00001996 740A00001B   IL_0106: castclass uint8[]*/
		/* 0x0000199B 6F4F00000A   IL_010B: callvirt  instance void [mscorlib]System.Security.Cryptography.SymmetricAlgorithm::set_Key(uint8[])*/
		/* 0x000019A0 1104         IL_0110: ldloc.s   V_4*/
		/* 0x000019A2 7456000001   IL_0112: castclass [mscorlib]*/
		$inst_aes_setup = {
			13 ??
			11 ??
			74 [4]
			14
			FE ??
			13 ??
			11 ??
			2C ??
			17
			13 ??
			2B ??
			19
			2B ??
			11 ??
			74 [4]
			08
			74 [4]
			6F [4]
			11 ??
			74
		}
		/* 0x00001A28 06            IL_0198: ldloc.0*/
		/* 0x00001A29 740A00001B    IL_0199: castclass uint8[]*/
		/* 0x00001A2E 7E0C000004    IL_019E: ldsfld    char[] Jr37M.o9G3E::F*/
		/* 0x00001A33 20E9000000    IL_01A3: ldc.i4    233*/
		/* 0x00001A38 7E0C000004    IL_01A8: ldsfld    char[] Jr37M.o9G3E::F*/
		/* 0x00001A3D 20E9000000    IL_01AD: ldc.i4    233*/
		/* 0x00001A42 93            IL_01B2: ldelem.u2*/
		/* 0x00001A43 7E4F000004    IL_01B3: ldsfld    char[] Go58W.b4LYd::o*/
		/* 0x00001A48 2002010000    IL_01B8: ldc.i4    258*/
		/* 0x00001A4D 93            IL_01BD: ldelem.u2*/
		/* 0x00001A4E 61            IL_01BE: xor*/
		/* 0x00001A4F 209C000000    IL_01BF: ldc.i4    156*/
		/* 0x00001A54 5F            IL_01C4: and*/
		/* 0x00001A55 9D            IL_01C5: stelem.i2*/
		$inst_str_ref = {
			06
			74 [4]
			7E [4]
			20 [4]
			7E [4]
			20 [4]
			93
			7E [4]
			20 [4]
			93
			61
			20 [4]
			5F
			9D
		}
	condition:
		any of ($str_*) or
		2 of ($inst_*)
}
