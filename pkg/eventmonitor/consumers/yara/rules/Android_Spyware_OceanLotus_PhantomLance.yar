rule Android_Spyware_OceanLotus_PhantomLance {
    meta:
        description = "Detects OceanLotus (APT32) PhantomLance Android spyware APK files based on C2 domains and specific internal DEX framework invocations"
        author = "Serhii Kocherhan"
        date = "2026-09-06"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "4eca4c72-2e84-465f-ab25-7874b4e8b586"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "18e15792494ff98e24b7e5c57b897b96"

    strings:
        // APK/ZIP Magic Header ("PK\x03\x04")
        $zip_magic = { 50 4B 03 04 }

        // Android Executable Bytecode File Magic ("dex\n035\0")
        $dex_magic = { 64 65 78 0A 30 33 35 00 }

        // PhantomLance C2 Infrastructure Domains
        $c2_1 = "file.log4jv.info" ascii wide nocase
        $c2_2 = "log.osloger.biz" ascii wide nocase
        $c2_3 = "news.sqllitlever.info" ascii wide nocase

        // Low-level Internal Android & Conscrypt Framework Invocation Strings
        $method_1 = "Landroid/net/NetworkInfo$State;->values" ascii
        $method_2 = "Landroid/os/SystemProperties;->addChangeCallback" ascii
        $method_3 = "Landroid/os/SystemProperties;->getLong" ascii
        $method_4 = "Lcom/android/org/conscrypt/OpenSSLCipher$Mode;->values" ascii
        $method_5 = "Lcom/android/org/conscrypt/OpenSSLCipher$Padding;->values" ascii

        // Internal Conscrypt / SystemProperties string fragments (fallback)
        $string_conscrypt = "com/android/org/conscrypt/OpenSSLCipher" ascii
        $string_sysprop   = "android/os/SystemProperties" ascii

    condition:
        // Must be a valid APK container or DEX file within reasonable size
        ($zip_magic at 0 or $dex_magic) and filesize < 50MB and
        (
            // Primary Match: Any of the known C2 domains
            any of ($c2_*)
            or
            // Secondary Match: 3 or more of the specific internal Android/Conscrypt methods
            3 of ($method_*)
            or
            // Fallback Match: Combination of C2 string fragments + internal cryptography reflection
            (
                1 of ($c2_*) and 
                ($string_conscrypt or $string_sysprop)
            )
        )
}