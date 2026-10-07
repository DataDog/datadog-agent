rule BrowserExtensionLoader {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-11-08"
        description = "Detects Chrome/Edge browser extension loader"
        yarahub_uuid = "9aa9f2aa-f3e3-4068-a7ca-17b89cfd03d4"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6c51dde7b67ecdd5b5ba4db58542a0a4"
    
    strings:
        $proc_chrome = "chrome.exe" wide ascii
        $proc_edge = "msedge.exe" wide ascii
        
        $cmd_kill = "taskkill /IM %s /F" wide ascii
        $cmd_load = "--load-extension" wide ascii
        $cmd_restore = "--restore-last-session" wide ascii
        
        $path_chrome = "\\AppData\\Local\\Google\\Chrome" wide ascii
        $path_chrome_beta = "\\AppData\\Local\\Google\\Chrome Beta" wide ascii
        $path_edge = "\\AppData\\Local\\Microsoft\\Edge" wide ascii
        
    condition:
        uint16(0) == 0x5a4d and
        (any of ($proc*) and 
        all of ($cmd*) and 
        any of ($path*))
}