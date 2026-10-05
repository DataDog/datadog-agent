rule GenericUnknownDotnetRAT {
	meta:
		author = "Still"
		component_name = "UnknownDotnetRAT"
		date = "2025-11-07"
		description = "attempts to match instructions found in .NET backdoors related to XWorm/VenomRAT"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "f40e0a193d43d43bad1aaf2657868aaa"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "bf8005e7-0b51-49ba-9522-ec9b2a09239f"
	strings:
		/* 0x000000AD 3858FFFFFF   IL_00AD: br        IL_000A */
		/* 0x000000B2 288F000006   IL_00B2: call      class [mscorlib]System.Random ProcessHelper::NewRandom$PST0600008F() */
		/* 0x000000B7 20E8030000   IL_00B7: ldc.i4    1000 */
		/* 0x000000BC 2088130000   IL_00BC: ldc.i4    5000 */
		/* 0x000000C1 2890000006   IL_00C1: call      int32 ProcessHelper::GetRandomValue$PST06000090(class [mscorlib]System.Random, int32, int32) */
		/* 0x000000C6 287A000006   IL_00C6: call      void ProcessHelper::ThreadSleep$PST0600007A(int32) */
		$inst_thread_sleep = {
			38 [4]
			28 [4]
			20 E8 03 00 00
			20 88 13 00 00
			28 [4]
			28
		}

		/* 0x00000053 7E3D000004    IL_0053: ldsfld    class [System]System.Net.Sockets.Socket CommsHelper::_socket*/
		/* 0x00000058 2000C80000    IL_0058: ldc.i4    51200*/
		/* 0x0000005D 28A2000006    IL_005D: call      void CommsHelper::SetSendBufferSize$PST060000A2(class [System]System.Net.Sockets.Socket, int32)*/
		/* 0x00000062 7E3D000004    IL_0062: ldsfld    class [System]System.Net.Sockets.Socket CommsHelper::_socket*/
		/* 0x00000067 7E33000004    IL_0067: ldsfld    string Settings::Hostname*/
		/* 0x0000006C 7E34000004    IL_006C: ldsfld    string Settings::PortValue*/
		/* 0x00000071 28A3000006    IL_0071: call      int32 CommsHelper::ConvertToInt$PST060000A3(string)*/
		$inst_setup_socket = {
			7E [4]
			20 00 C8 00 00
			28 [4]
			7E [4]
			7E [4]
			7E [4]
			28
		}

		/* 0x00007491 0A            IL_0141: stloc.0 */
		/* 0x00007492 06            IL_0142: ldloc.0 */
		/* 0x00007493 14            IL_0143: ldnull */
		/* 0x00007494 28A9000006    IL_0144: call      class [mscorlib]System.Random '\u202c\u206a\u200f\u206e\u202b\u200f\u202a\u206e\u200d\u202e\u200c\u202c\u202b\u200b\u202e\u202d\u202d\u206f\u206d\u206b\u202c\u202d\u206a\u202c\u202e\u200f\u206b\u200c\u202d\u202a\u206e\u206f\u206c\u206a\u202d\u202a\u202b\u200d\u200d\u202c\u202e'::'\u206f\u202a\u206b\u202d\u200c\u206f\u202d\u206c\u206e\u206c\u202c\u202a\u202e\u206a\u200c\u200e\u202e\u200d\u202e\u202a\u200d\u206f\u206f\u200f\u206c\u202d\u206f\u200c\u200c\u200c\u202e\u202a\u200f\u200c\u202a\u202e\u206e\u200b\u206b\u200c\u202e$PST060000A9'() */
		/* 0x00007499 2010270000    IL_0149: ldc.i4    10000 */
		/* 0x0000749E 20983A0000    IL_014E: ldc.i4    15000 */
		/* 0x000074A3 28AA000006    IL_0153: call      int32 '\u202c\u206a\u200f\u206e\u202b\u200f\u202a\u206e\u200d\u202e\u200c\u202c\u202b\u200b\u202e\u202d\u202d\u206f\u206d\u206b\u202c\u202d\u206a\u202c\u202e\u200f\u206b\u200c\u202d\u202a\u206e\u206f\u206c\u206a\u202d\u202a\u202b\u200d\u200d\u202c\u202e'::'\u206b\u202b\u202e\u206a\u206e\u202e\u200f\u200b\u200c\u206d\u202c\u200f\u200f\u202c\u206b\u202a\u202e\u202d\u200d\u200c\u202a\u206d\u200e\u206e\u200e\u206c\u202a\u206b\u202d\u200b\u200f\u202c\u202e\u200b\u200e\u202c\u206c\u206a\u200c\u200c\u202e$PST060000AA'(class [mscorlib]System.Random, int32, int32) */
		/* 0x000074A8 28A9000006    */
		$inst_timer = {
			0A
			06
			14
			28 [4]
			20 10 27 00 00
			20 98 3A 00 00
			28 [4]
			28
		}
	condition:
		any of them
}
