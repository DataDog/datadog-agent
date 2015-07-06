package network

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"
)

func TestWindowsNetworkParser(t *testing.T) {
	// Berk berk berk, this is the output from a Surface Pro I had at hand :s
	// TODO FIXME : put that in a file and use multiple files
	bin_out, err := ioutil.ReadFile("./ipconfig_test_sample.txt")
	if err != nil {
		t.Errorf("This test is a complete disaster. We can't even read the " +
			"./ipconfig_test_sample.txt file. Let's give up !\n\n" +
			err.Error())
		return
	}
	ip_config_output := string(bin_out)

	expected := map[string]interface{}{
		"ipaddress":   "10.42.30.215",
		"macaddress":  "4C-0B-BE-2A-94-46",
		"ipaddressv6": "fe80::6d56:b5b1:7fe0:b097%3",
	}
	res, err := parseIpConfig(ip_config_output)

	if expected["ipaddress"] == res["ipaddress"] {
		fmt.Println("OK1")
	}
	if expected["macaddress"] == res["macaddress"] {
		fmt.Println("OK2")
	}
	if expected["ipaddressv6"] == res["ipaddressv6"] {
		fmt.Println("OK3")
	}

	if !reflect.DeepEqual(res, expected) {
		if err != nil {
			t.Errorf(err.Error())
			return
		}

		// Let's compute an explicit error message
		msg := "variable\t\tresult\t\t\t\texpected\n"
		for k, v := range expected {
			if resk, isastring := res[k].(string); isastring {
				val := v.(string)
				msg += k + ":\t\t " + resk + " \t\t\t " + val + " \n"
				fmt.Println(v)
			} else {
				t.Errorf("Error during the test. Even though it doesn't look " +
					"like it's the case, getNetworkInfo() is expected " +
					"to return a imap[string]string.")
				return
			}
		}
		t.Errorf("getNetworkInfo() for Windows: TEST FAILURE \n\n" + msg)
	}
	return
}
