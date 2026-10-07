rule PaaS_SpearPhishing_Feb23
{

    meta:
	author = "Alexander Hatala (@AlexanderHatala)"
	description = "Detects targeted spear phishing campaigns using a private PaaS based on filenames."
	date = "2023-02-11"
	tlp = "CLEAR"
	yarahub_reference_md5 = "084b4397d2c3590155fed50f0ad9afcf"
	yarahub_uuid = "2c4733fc-3ec7-45db-adae-1a396ba8d4ae"
	yarahub_license = "CC BY 4.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"
	yarahub_author_twitter = "@AlexanderHatala"

    strings:
        $file1 = "saved_resource.html"
        $file2 = "/antibots7/"
        $file3 = "infos.php"
        $file4 = "config00.php"
        $file5 = "config0.php"
        $file6 = "personal.php"
        $file7 = "Email.php"
        
    condition:
        all of them
}
