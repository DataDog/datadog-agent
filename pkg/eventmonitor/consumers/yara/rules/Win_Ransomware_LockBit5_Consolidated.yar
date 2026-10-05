rule Win_Ransomware_LockBit5_Consolidated {
    meta:
        description = "Consolidated rule to detect LockBit 5.0 (Erebus) Ransomware payloads and notes"
        author = "RussianPanda, S2W, ransomware.live (Consolidated)"
        version = "1.0"
        
        // --- Mandatory YARAhub / YARAify Fields ---
        date = "2026-07-21"
        yarahub_uuid = "0afdb09c-b4d8-4c3f-bb07-f445606fbab5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f79ea684b3d459cf3f9d93dac0818ad5"
        
        // --- Reference Hashes ---
        sha1_1 = "d45ad2f115303e6e13cad24d3844155812c65c88" 
        sha1_2 = "e032a960e7ead94366a4da3b64c5e7d2b61c6a0c" 
        sha1_3 = "fda895c21e11f8af195420d5b556604e8aea466d" 
        sha256 = "7ea5afbc166c4e23498aa9747be81ceaf8dad90b8daa07a6e4644dc7c2277b82"

    strings:
        // --- Ransom Note & Branding ---
        $note1 = "LockBit 5" ascii wide nocase
        $note2 = "Erebus" ascii wide nocase
        $note3 = "lockbit5" ascii wide nocase

        // --- Cryptographic Constants ---
        // ChaCha20 algorithm constant  
        $chacha = "expand 32-byte k" fullword ascii wide nocase 
        // SHA512 Custom IV
        $sha512_customIV = { 10 C9 BD F2 67 E6 09 6A }

        // --- API Hashing & Encryption Logic ---
        // General API resolve (32 and 64 bit)
        $api_resolve_gen = { 
            (80 FA | 41 80 F9) 1A [0-10]                  
            [0-1] 8D ?? ?? ?? 00 00        
            [0-1] 0F AF ??                 
            [0-3] 01 ??                    
            [0-1] 89 ??                    
            [0-1] 81 ?? ?? ?? 00 00        
        }
        // Specific x64 API resolve variant
        $api_resolve_x64 = {
            44 31 ??                 
            [0-1] 8D ?? ?? ?? 00 00       
            [0-1] 0F AF ??                
            [0-3] 01 ??                   
            [0-1] 89 ??                   
            [0-1] 81 ?? ?? ?? 00 00       
        }
        $encrypt_code1 = { 48 81 ?? 00 00 00 05 } 
        $encrypt_code2 = {                  
            48 8D ?? FF FF 7F 00          
            48 C1 ?? 14                     
            4? ?? F8 FF FF FF FF 07 00 00  
            4? 21 ??                        
            4? 8D ?? ??                   
            4? 83 ?? 60                    
        }

        // --- Geofencing & Country Checks (0x0419 = Russia, 0x00C9 = Russian Federation) ---
        $check_country_code1 = { FF D0 [0-10] 3D 19 04 00 00 } 
        $check_country_code2 = { FF D0 [0-10] 3D C9 00 00 00 } 

        // --- Core Binary Mechanics (RussianPanda) ---
        $mech1 = { C6 41 0F 00 0F B6 ?? 33 ?? 89 }
        $mech2 = { 0F B6 ?? 0F C1 ?? 18 31 } 
        $mech3 = { 83 ?? 02 83 ?? 0F D0 84 ?? ?? 00 00 00 }

    condition:
        // Trigger on text-based ransom notes OR Windows Portable Executables (PE)
        any of ($note*) or 
        (
            uint16(0) == 0x5A4D and 
            filesize < 5MB and 
            (
                all of ($mech*) or
                ($api_resolve_gen and $chacha and any of ($check_country_code*)) or
                (all of ($encrypt_code*) and $sha512_customIV and any of ($check_country_code*) and any of ($api_resolve*))
            )
        )
}