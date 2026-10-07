rule OleDownloader
{
    meta:
        author = "Madhav"
        description = "This is a ole file which is accessing some url"
        date = "2025-05-09"
	yarahub_reference_md5 = "bff78436218e2ade64470c183020168e"
	yarahub_uuid = "8623936b-908a-4e37-b140-c5a947d00324"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "\"h\", \"t\", \"t\", \"p\", \":\", \"/\", \"/\""
        $mal2 = "\"M\", \"S\", \"X\", \"M\", \"L\", \"2\", \".\", \"X\", \"M\", \"L\", \"H\", \"T\", \"T\", \"P\""
        $mal3 = ".exe"
        $mal4 = "\"A\", \"D\", \"O\", \"D\", \"B\", \".\", \"S\", \"t\", \"r\", \"e\", \"a\", \"m\""
        $mal5 = "winmgmts:\\\\.\\root\\cimv2"
        $mal6 = "\"c\" & \"m\" & \"d /\" & \"c \""
        $autoopen = "Sub AutoOpen()"
        $docopen = "Sub Document_Open()"
    
    condition:
        $mal1 and $mal2 and $mal3 and $mal4 and $mal5 and $mal6 and $autoopen and $docopen
}