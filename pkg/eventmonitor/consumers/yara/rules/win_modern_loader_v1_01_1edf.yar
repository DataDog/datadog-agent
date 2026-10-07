rule win_modern_loader_v1_01_1edf {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-12-08"
        description               = "matches unpacked ModernLoader samples"
        hash_md5                  = "c6897769c0af03215d61e8e63416e5fc"
        hash_sha1                 = "12261b515dabba8a5bb0daf0a904792d3acd8f9b"
        hash_sha256               = "ceae593f359a902398e094e1cdbc4502c8fd0ba6b71e625969da6df5464dea95"
        malpedia_family           = "win.modern_loader"
        tlp                       = "TLP:WHITE"
        version                   = "v1.01"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "c6897769c0af03215d61e8e63416e5fc"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "1edff524-1b52-494c-8d61-3daf5998b8cc"

    strings:
        $log_01 = "[DEBUG] Download & Execute Content: <" wide
        $log_02 = "[DEBUG] Execute Content: <" wide
        $log_03 = "[DEBUG] Init Completed Response: <" wide
        $log_04 = "[DEBUG] Listen Response: <" wide
        $log_05 = "[DEBUG] Task Completed Response: <" wide
        $log_06 = "[DEBUG] Task Failed Response: <" wide
        $log_07 = "[DEBUG] Task Result: <" wide
        $log_08 = "[ERROR] Creating Request Failed" wide
        $log_09 = "[ERROR] Listen Failed" wide
        $log_10 = "[ERROR] No available tasks or tasks parsing error" wide
        $log_11 = "[ERROR] Reading Response Failed" wide

        $fingerprint_1 = "\"AntiVirus\":\"N/A\"," wide
        $fingerprint_2 = "\"CORP\":\"N/A\"," wide
        $fingerprint_3 = "\"Network PCs\":\"N/A\"}" wide
        $fingerprint_4 = "\"RDP\":\"" wide
        $fingerprint_5 = "\"Role\":\"Admin\"," wide
        $fingerprint_6 = "\"Role\":\"User\"," wide
        $fingerprint_7 = "\"Total Space\":\"" wide
        $fingerprint_8 = "\"Version\":\"" wide

        $varia_01 = "%XBoxLive%" wide
        $varia_02 = "AddressWidth" wide
        $varia_03 = "C:\\Users\\Public\\Documents\\Data\\hidden_service\\hostn" wide
        $varia_04 = "Download & Execute" wide
        $varia_05 = "HKEY_LOCAL_MACHINE\\HARDWARE\\DESCRIPTION\\SYSTEM\\CENT" wide
        $varia_06 = "ProcessorNameString" wide
        $varia_07 = "RALPROCESSOR\\0" wide
        $varia_08 = "Win32_ComputerSystem" wide
        $varia_09 = "partofdomain" wide
        $varia_10 = "root\\SecurityCenter2" wide

        $sql_1 = "SELECT * FROM AntivirusProduct" wide
        $sql_2 = "SELECT * FROM Win32_DisplayConfiguration" wide
        $sql_3 = "SELECT Caption FROM Win32_OperatingSystem" wide
        $sql_4 = "SELECT UUID FROM Win32_ComputerSystemProduct" wide
        $sql_5 = "select * from Win32_Processor" wide

    condition:
        uint16(0) == 0x5A4D and
        (
            30 of them
        )
}
