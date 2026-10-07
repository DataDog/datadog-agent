rule Hunt_PowerShell_Stager_UUID_Decompressor {
    meta:
        description = "Detects PowerShell stagers that use custom C# MP4/UUID payload extraction, hidden window invocations, and environment sanity checks"
        author = "Serhii Kocherhan"
        date = "2026-08-26"
        yarahub_uuid = "7ebe1cc2-ecb9-4b18-9910-4ae12a3e5654"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // Environment / Sanity Check Base64 Strings
        // "KkNMRUFOKg==" -> "*CLEAN*"
        $b64_clean = "KkNMRUFOKg==" ascii wide
        // "LmNoX2ZlM2UxNmZhOTQwNg==" -> ".ch_fe3e16fa9406"
        $b64_mutex = "LmNoX2ZlM2UxNmZhOTQwNg==" ascii wide

        // Distinct C# Win32 P/Invoke definition block to hide the console window
        $csharp_hideconsole = "public class __HideConsole" ascii wide
        $win32_showwindow    = "ShowWindow(IntPtr" ascii wide

        // Embedded payload decompressor inner code artifacts (Base64-encoded C# class)
        $cs_class_name  = "___X194ODY2NTgxM2U" ascii wide
        $cs_uuid_method = "FindUuidPayload" ascii wide
        $cs_magic_bytes = "205,129,92,186" ascii wide

        // Process execution and cleanup patterns
        $ps_exec_flag = "-ExecutionPolicy" ascii wide nocase
        $clean_runmru = "Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\RunMRU" ascii wide nocase

    condition:
        // Match if the core C# payload decompressor artifacts are present
        (
            $cs_uuid_method and $cs_magic_bytes
        )
        or
        (
            // Or match a combination of the environment checks, P/Invoke structures, and execution behaviors
            all of ($b64_*) or
            (
                $csharp_hideconsole and 
                $win32_showwindow and 
                any of ($cs_class_name, $clean_runmru, $ps_exec_flag)
            )
        )
}