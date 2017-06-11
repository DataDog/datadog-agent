package flare

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// AskForEmail asks for the user's email
func AskForEmail() string {
	var email string
	email, _ = askForInput("Please enter your email: ", "")
	return email
}

// AskForConfirmation asks for the user's confirmation to send the flare
func AskForConfirmation(input string) bool {
	response, e := askForInput(input, "")
	if e != nil {
		return false
	}
	if response == "y" || response == "Y" {
		return true
	}
	return false
}

// 'Are you sure you want to continue [y/N]? '

func askForInput(before string, after string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	if before != "" {
		fmt.Println(before)
	}
	text, err := reader.ReadString('\n')
	if after != "" {
		fmt.Println(after)
	}
	text = strings.TrimSpace(text)
	return text, err
}
