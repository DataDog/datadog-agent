rule AutoIt_DLL_Carrier_RAT {
    meta:
        description = "AutoIt3 RAT - encrypted AU3!EA06 bytecode appended to signed IconLib.dll carrier"
        author = "BlackCatOfficial"
        date = "2026-05-18"
        yarahub_uuid = "c754140f-f078-4239-abcd-95d5f5138262"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "cc042e32be5000618789f11fd8358c17"
        reference = "https://bazaar.abuse.ch/sample/59386bf1258fff2b4ebccdca764aa9c10d50d7df75ecfb5308e4474008bd17d2/"
        cert_thumbprint = "44061cf10437607658791221e105654f9a1de00492460146f6836c69d1c75dad"
        c2_ip = "193.124.33.180"
        c2_telegram = "t.me/s/apinono"

    strings:
        $au3_magic = "AU3!EA06" ascii
        $iconlib_dll = "IconLib" ascii nocase
        $ntfs_hidden1 = "config.." ascii
        $ntfs_hidden2 = "logdata.." ascii
        $ntfs_hidden3 = "diagnostic.." ascii
        $c2_telegram = "t.me/s/apinono" ascii
        $c2_ip = "193.124.33.180" ascii
        $index_exe = "Index.exe" ascii
        $errorhandler = "ErrorHandler.cmd" ascii

    condition:
        uint16(0) == 0x5A4D and
        filesize > 4MB and
        #au3_magic and
        2 of ($ntfs_hidden1, $ntfs_hidden2, $ntfs_hidden3) and
        any of ($iconlib_dll, $c2_telegram, $c2_ip, $index_exe, $errorhandler)
}