rule Python_NET_RAT_Multistage_Artifacts
{
	meta:
        author = "BitAutopsy"
        description = "Detects a multi-stage Python/.NET RAT and related persistence, scriptlet, watchdog and guard.pyd artifacts"
        date = "2026-08-30"

        yarahub_reference_md5 = "ae9e3c97e5262816e47b12b4d9a3c756"
		
		sample_sha256 = "c1a052374eb088ab3d049fd4eb8c8b4b9668bdfc724b8e6d3f74de42eb22c8ab"

        yarahub_uuid = "cb6de1e6-0a07-489d-85f2-35254ce6e15f"

        yarahub_license = "CC0 1.0"

        yarahub_rule_matching_tlp = "TLP:WHITE"

        yarahub_rule_sharing_tlp = "TLP:WHITE"
	
	strings:
		$sfx_1 = { 73 76 63 68 6F 73 74 2E 70 79 63 } // svchost.pyc
		$sfx_2 = { E5 AE 88 E6 8A A4 E9 80 BB E8 BE 91 E5 B7 B2 E5 AE 8C E5 85 A8 E7 94 B1 20 67 75 61 72 64 69 61 6E 2E 70 79 63 20 E8 B4 9F E8 B4 A3 } // guardian.pyc handles watchdog logic
		
		$generic_1 = { 77 69 6E 72 65 67 } // winreg
		$generic_2 = { 53 6F 66 74 77 61 72 65 5C 4D 69 63 72 6F 73 6F 66 74 5C 57 69 6E 64 6F 77 73 5C 43 75 72 72 65 6E 74 56 65 72 73 69 6F 6E 5C 52 75 6E } // Software\Microsoft\Windows\CurrentVersion\Run
		$generic_3 = { 53 6f 66 74 77 61 72 65 5c 4d 69 63 72 6f 73 6f 66 74 5c 57 69 6e 64 6f 77 73 5c 43 75 72 72 65 6e 74 56 65 72 73 69 6f 6e 5c 52 75 6e 4f 6e 63 65 } // Software\Microsoft\Windows\CurrentVersion\RunOnce
		$generic_4 = { 49 6E 70 72 6F 63 53 65 72 76 65 72 33 32 } // InprocServer32
		$generic_5 = { 53 63 72 69 70 74 6C 65 74 55 52 4C } // ScriptletURL
		$generic_6 = { 57 53 63 72 69 70 74 2E 53 68 65 6C 6C } // WScript.Shell
		$generic_7 = { 41 63 74 69 76 65 58 4F 62 6A 65 63 74 } // ActiveXObject
		$generic_8 = { 41 6D 73 69 53 63 61 6E 42 75 66 66 65 72 } // AmsiScanBuffer
		$generic_9 = { 45 74 77 45 76 65 6E 74 57 72 69 74 65 } // EtwEventWrite
		
		$svchost_1 = { E5 A6 82 E6 9E 9C E5 B0 91 E4 BA 8E 20 32 20 E4 B8 AA E8 BF 9B E7 A8 8B EF BC 8C E9 87 8D E5 90 AF } // restart if fewer than 2 processes
		$svchost_2 = { E6 A3 80 3F 70 79 74 68 6F 6E 77 2E 65 78 65 20 E6 98 AF E5 90 A6 E5 9C A8 E8 BF 90 E8 A1 8C } // check whether pythonw.exe is running
		
		$bat_1 = { 70 79 74 68 6F 6E 77 2E 65 78 65 } // pythonw.exe
		$bat_2 = { 73 76 63 68 6F 73 74 2E 70 79 63 } // svchost.pyc
		$bat_3 = { 25 7E 64 70 30 } // %~dp0
		$bat_4 = { 73 74 61 72 74 20 22 22 20 2F 42 } // start "" /B
		
		$sct_1 = { E6 A3 80 3F 70 79 74 68 6F 6E 77 2E 65 78 65 20 E6 98 AF E5 90 A6 E5 9C A8 E8 BF 90 E8 A1 8C } // check whether pythonw.exe is running
		$sct_2 = { E5 A6 82 E6 9E 9C E5 B0 91 E4 BA 8E 20 32 20 E4 B8 AA E8 BF 9B E7 A8 8B EF BC 8C E9 87 8D E5 90 AF } // restart if fewer than 2 processes
		$sct_3 = { 73 76 63 68 6F 73 74 2E 70 79 63 } // svchost.pyc
		$sct_4 = { 74 61 73 6B 6C 69 73 74 20 2F 46 49 } // tasklist /FI
		$sct_5 = { 41 63 74 69 76 65 58 4F 62 6A 65 63 74 28 22 57 53 63 72 69 70 74 2E 53 68 65 6C 6C 22 29 } // ActiveXObject("WScript.Shell")
		$sct_6 = { 46 35 36 46 36 46 44 44 2D 41 41 39 44 2D 34 36 31 38 2D 41 39 34 39 2D 43 31 42 39 31 41 46 34 33 42 31 41 } // F56F6FDD-AA9D-4618-A949-C1B91AF43B1A
		
		$guard_1 = { 66 69 6C 65 47 75 61 72 64 20 2D 2D 20 45 78 74 72 65 6D 65 20 70 72 6F 63 65 73 73 2D 68 61 6E 67 69 6E 67 20 66 69 6C 65 2F 64 69 72 65 63 74 6F 72 79 20 6C 6F 63 6B 20 75 74 69 6C 69 74 79 } // fileGuard -- Extreme process-hanging file/directory lock utility
		$guard_2 = { 66 69 6C 65 47 75 61 72 64 } // fileGuard
		$guard_3 = { 50 72 6F 6A 46 53 } // ProjFS
		$guard_4 = { 46 69 6C 65 50 72 6F 74 65 63 74 6F 72 28 76 65 72 62 6F 73 65 3D 46 61 6C 73 65 2C 20 73 74 6F 70 5F 74 6F 6B 65 6E 5F 70 61 74 68 3D 4E 6F 6E 65 2C 20 62 6C 6F 63 6B 5F 6D 6F 64 65 3D 42 4C 4F 43 4B 5F 57 41 49 54 2C 20 72 65 63 75 72 73 69 76 65 5F 73 63 61 6E 5F 74 72 61 70 3D 54 72 75 65 29 } // FileProtector(verbose=False, stop_token_path=None, block_mode=BLOCK_WAIT, recursive_scan_trap=True)
		$guard_5 = { 65 78 65 63 75 74 65 5F 73 68 65 6C 6C 63 6F 64 65 28 64 61 74 61 2C 20 78 6F 72 5F 6B 65 79 } // execute_shellcode(data, xor_key
		$guard_6 = { 73 6C 65 65 70 5F 78 6F 72 3D } // sleep_xor=
		$guard_7 = { 65 78 65 63 75 74 65 5F 73 68 65 6C 6C 63 6F 64 65 3A 20 66 61 69 6C 65 64 20 74 6F 20 69 6E 73 74 61 6C 6C 20 41 50 49 20 68 6F 6F 6B 73 } // execute_shellcode: failed to install API hooks
		$guard_8 = { 68 6F 6F 6B 20 53 6C 65 65 70 20 74 6F 20 58 4F 52 2D 65 6E 63 72 79 70 74 20 6E 6F 6E 2D 44 4C 4C 20 52 58 20 70 61 67 65 73 20 61 72 6F 75 6E 64 20 65 61 63 68 20 73 6C 65 65 70 20 69 6E 74 65 72 76 61 6C } // hook Sleep to XOR-encrypt non-DLL RX pages around each sleep interval
		
	condition:
		(
			$sfx_1 and 
			$sfx_2			
		)
		or
		(
			$generic_1 and
			(
				$generic_2 or
				$generic_3
			)
			and
			(
				$generic_4 and
				$generic_5
			)
			and
			(
				$generic_6 or
				$generic_7
			)
			and
			(
				$generic_8 or
				$generic_9
			)
		)
		or
		(
			$svchost_1 and
			$svchost_2
		)
		or
		(
			$bat_1 and
			$bat_2 and
			(
				$bat_3 or
				$bat_4
			)
		)
		or
		(
			(
				$sct_1 and
				$sct_2
			)
			and
			(
				$sct_3 or
				$sct_4
			)
			and
			(
				$sct_5 or
				$sct_6
			)
		)
		or
		(
			$guard_1
			and
			$guard_2
			and
			$guard_3
			and
			(
				$guard_4 or
				(
					$guard_5
					and
					$guard_6
				)
				or
				$guard_7
				or
				$guard_8
				
			)
			and
			uint16(0) == 0x5A4D
		)
		
}