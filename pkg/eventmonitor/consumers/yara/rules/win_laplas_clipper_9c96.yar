rule win_laplas_clipper_9c96 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-11-09"
        description               = "detects unpacked Laplas Clipper"
        hash1_md5                 = "3afb4573dea2dbac4bb5f1915f7a4dce"
        hash1_sha1                = "9ad8b880f3ab35f0d1a7fe46d9d8e0bea36e0d14"
        hash1_sha256              = "52901dc481d1be2129725e3c4810ae895f9840e27a1dce69630dedcf71b6c021"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "3afb4573dea2dbac4bb5f1915f7a4dce"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
	yarahub_uuid              = "5f272188-cabb-441a-8278-b9b82fe4d653"


    strings:
        $func_names_0 = "main.request"
        $func_names_1 = "main.setOnline"
        $func_names_2 = "main.getRegex"
        $func_names_3 = "main.getAddress"
        $func_names_4 = "main.waitOpenClipboard"
        $func_names_5 = "main.clipboardRead"
        $func_names_6 = "main.clipboardWrite"
        $func_names_7 = "main.startHandler"
        $func_names_8 = "main.isRunning"
        $func_names_9 = "main.main"
        $func_names_10 = "main.isStartupEnabled"
        $func_names_11 = "main.decrypt"
        $func_names_12 = "main.existsPath"
        $func_names_13 = "main.getPid"
        $func_names_14 = "main.writePid"
        $func_names_15 = "main.enableStartup"
        $func_names_16 = "main.copyFile"
        $func_names_17 = "main.clipboardWrite.func1"
        $func_names_18 = "main.init"

        $startup_0 = "/sc"
        $startup_1 = "/ri"
        $startup_2 = "/st"
        $startup_3 = "/tr"
        $startup_4 = "/tn"
        $startup_5 = "/create"
        $startup_6 = "/C"
        $startup_7 = "once"
        $startup_8 = "cmd.exe"
        $startup_9 = "9999:59"
        $startup_10 = "00:00"

        $request_0 = "http://"
        $request_1 = "/bot/"
        $request_2 = "key="

    condition:
        uint16(0) == 0x5A4D and
        17 of ($func_names_*)  and
        9 of ($startup_*) and
        all of ($request_*)
}
