package flare

import (
	"bufio"
	"fmt"
	"os"
)

// AskForEmail asks for the user's email
func AskForEmail() string {
	var email string
	email, _ = askForInput("Please enter your email: ", "")
	return email
}

// AskForConfirmation asks for the user's confirmation to send the flare
func AskForConfirmation() bool {
	response, _ := askForInput("Are you sure you want to upload a flare? [Y/N]", "")
	if response == "y" || response == "Y" {
		return true
	}
	return false
}

func askForInput(before string, after string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	if before != "" {
		fmt.Println(before)
	}
	text, err := reader.ReadString('\n')
	if after != "" {
		fmt.Println(after)
	}
	return text, err
}
