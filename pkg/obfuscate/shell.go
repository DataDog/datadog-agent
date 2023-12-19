// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SHICore is the main parsing primitive
type SHICore struct {
	IsShellCommand bool
	ShellCommand   string
	SpawnShell     bool
	CommandArgs    []string
}

// ObfuscatedSlice is a slice of a string representing an obfuscated command
type ObfuscatedSlice struct {
	start int
	end   int
}

// ErrCommandExecParse is returned when the command failed to be parsed
var (
	ErrCommandExecParse = fmt.Errorf("failed to parse command")
)

// Shared context
var (
	regexParamDeny  = regexp.MustCompile(`^-{0,2}(?:p(?:ass(?:w(?:or)?d)?)?|api_?key|secret|a(?:ccess|uth)_token|mysql_pwd|credentials|(?:stripe)?token)$`)
	regexEnvVars    = regexp.MustCompile(`([\d\w_]+)=.+`)
	allowedEnvVars  = []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH"}
	processDenyList = []string{"md5"}
)

// NewSHICore creates a context for the command
func NewSHICore(cmd string, isCmdExec bool) (SHICore, error) {
	commandStr := cmd
	shellCommand, spawnShell := false, false
	var commandArgs []string

	if !isCmdExec {
		shellCommand = true
		commandStr = cmd
	} else {
		// cmd.exec
		// The commandStr is a string representing a JSON array
		var args []string
		err := json.Unmarshal([]byte(commandStr), &args)
		if err != nil {
			return SHICore{}, ErrCommandExecParse
		}

		commandArgs = args

		// Literal command string
		commandStr = strings.Join(args, " ")

		// Detect if the first argument is a shell binary (attempt to spawn a shell)
		// This is a heuristic
		if len(args) > 0 && (strings.HasSuffix(args[0], "sh") || strings.HasSuffix(args[0], "bash.exe")) {
			shells := []string{
				// Linux shells
				"sh", "bash", "zsh", "csh", "ksh", "tcsh", "fish", "dash", "ash",
				// Windows shells
				"bash.exe",
			}

			// The arg can be prefixed with a path, so we only check the last part
			for _, shell := range shells {
				if strings.HasSuffix(args[0], shell) {
					spawnShell = true
					break
				}
			}
		}
	}

	return SHICore{
		IsShellCommand: shellCommand,
		ShellCommand:   commandStr,
		CommandArgs:    commandArgs,
		SpawnShell:     spawnShell,
	}, nil
}

// ObfuscateExecCommand obfuscates the given exec command that is represented as a JSON array.
// and returns the obfuscated command and the indices of tokens that were obfuscated as a string
// of space separated integers.
func (o *Obfuscator) ObfuscateExecCommand(cmd string) (string, string, error) {
	shi, err := NewSHICore(cmd, true)
	if err != nil {
		return "", "", err
	}

	// Obfuscate the command arguments
	obfuscatedArgs, obfuscatedIndices := obfuscateExecCommand(shi)

	// Rebuild the command string as a json array
	obfuscatedCommand, err := json.Marshal(obfuscatedArgs)
	if err != nil {
		return "", "", err
	}

	// Indices as a string with space separated integers
	indicesString := strings.Join(obfuscatedIndices, " ")

	return string(obfuscatedCommand), indicesString, nil
}

// ObfuscateShellCommand obfuscates the given shell command string.
// and returns the obfuscated command and the indices of tokens that were obfuscated as a string
// of space separated integers.
func (o *Obfuscator) ObfuscateShellCommand(cmd string) (string, string) {
	obfuscatedCmd, indices := obfuscateShellCommandString(cmd)

	// Indices as a string with space separated integers
	indicesString := strings.Join(indices, " ")

	return obfuscatedCmd, indicesString
}

// obfuscateExecCommand obfuscates the given exec command represented as the SHICore struct.
// returns the string array of obfuscated arguments
// and returns the obfuscated command and the indices of tokens that were obfuscated as a string
func obfuscateExecCommand(shi SHICore) ([]string, []string) {
	shellCommandObfuscatedIndex := -1
	var indices []string
	if shi.SpawnShell {
		// Find the shell command and obfuscate it
		shellCommandObfuscatedIndex = findShellCommand(shi.CommandArgs[1:])
		if shellCommandObfuscatedIndex != -1 {
			// Obfuscate the shell command
			cmd, indicesShell := obfuscateShellCommandString(shi.CommandArgs[shellCommandObfuscatedIndex])
			shi.CommandArgs[shellCommandObfuscatedIndex] = cmd

			// Add the shell command obfuscated indices to the global indices
			argIndexStr := strconv.Itoa(shellCommandObfuscatedIndex)
			for _, index := range indicesShell {
				indices = append(indices, argIndexStr+"-"+index)
			}
		}
	}

	// Create shell tokens from the command arguments
	tokens := make([]ShellToken, len(shi.CommandArgs))
	for i, arg := range shi.CommandArgs {
		tokens[i] = ShellToken{val: arg}
	}

	// Obfuscate the command arguments
	obfuscatedIndices := obfuscateCommandTokens(tokens, true, shellCommandObfuscatedIndex)

	// Rebuild the command arguments
	var obfuscatedArgs []string
	for _, token := range tokens {
		obfuscatedArgs = append(obfuscatedArgs, token.val)
	}

	// Merge the indices
	indices = append(indices, obfuscatedIndices...)

	return obfuscatedArgs, indices
}

// obfuscateShellCommandString obfuscates the given shell command string present in the SHICore struct.
// returns the obfuscated command and the indices of tokens that were obfuscated as a string
func obfuscateShellCommandString(cmd string) (string, []string) {
	// Parse the shell command
	tokens := parseShell(cmd)

	// Obfuscate the command tokens
	obfuscatedSlices := obfuscateShellCommandToken(tokens)

	var obfuscatedIndices []string
	offset := 0
	for _, slice := range obfuscatedSlices {
		// Remove from the cmd string the slice
		cmd = cmd[:slice.start-offset] + "?" + cmd[slice.end-offset:]

		// Add the obfuscated slice to the list of indices
		obfuscatedIndices = append(obfuscatedIndices, strconv.Itoa(slice.start-offset)+":"+strconv.Itoa(slice.end-slice.start))

		// Calculate the number of removed characters
		removedChars := slice.end - slice.start

		// Update the offset
		offset += removedChars - 1
	}

	return cmd, obfuscatedIndices
}

// obfuscateShellCommandToken obfuscates the given command arguments as a ShellToken array.
// returns the ShellToken array with obfuscated tokens and the indices of tokens that were obfuscated as a string array
func obfuscateShellCommandToken(tokens []ShellToken) []ObfuscatedSlice {
	// var obfuscatedArgsIndices []string
	var obfuscatedSlices []ObfuscatedSlice
	foundBinary := false

	for index := 0; index < len(tokens); index++ {
		token := (tokens)[index]

		if !foundBinary {

			if token.kind == VariableDefinition && !stringInSlice(token.val, allowedEnvVars) {
				// Detected a variable definition, obfuscate the value
				// Check if the next token is an equal sign and has a value
				if index+2 < len(tokens) && (tokens)[index+1].kind == Equal {
					// Get tokens that defines the value
					startValueToken := index + 2
					endValueToken, nbrGrabbed := grabFullArgument(tokens, index+2)

					// Replace the startValueToken with an obfuscated value
					if nbrGrabbed > 0 {
						obfuscatedSlices = append(obfuscatedSlices, ObfuscatedSlice{(tokens)[startValueToken].start, (tokens)[endValueToken].end})
					}

					// Increment the index to the end of the value
					index = endValueToken
				}
			} else if token.kind == Executable {
				// We found a binary, check if it is not on the deny list
				if stringInSlice(token.val, processDenyList) {
					// Remove every parameter until the end of the command
					i := index + 1
					for ; i < len(tokens) && (tokens[i].kind != Control && tokens[i].kind != Redirection); i++ {
						// If that's a parameter that have an equal sign (and possibly a value), this should be obfuscated as one value
						if i+1 < len(tokens) && (tokens)[i+1].kind == Equal {
							// Get tokens that defines the value
							startValueToken := i
							endValueToken, nbrGrabbed := grabFullArgument(tokens, i+2)

							// Remove the tokens that defines the value
							if nbrGrabbed > 0 {
								i += nbrGrabbed - 1 + 2
							} else {
								// If there is no value, we just skip the equal sign
								i++
							}

							obfuscatedSlices = append(obfuscatedSlices, ObfuscatedSlice{(tokens)[startValueToken].start, (tokens)[endValueToken].end})

						} else {
							// Get tokens that defines the value
							startValueToken := i
							endValueToken, nbrGrabbed := grabFullArgument(tokens, i)

							if nbrGrabbed > 0 {
								obfuscatedSlices = append(obfuscatedSlices, ObfuscatedSlice{tokens[startValueToken].start, tokens[endValueToken].end})
								i += nbrGrabbed - 1
							}
						}
					}

					// Skip all obfuscated tokens
					index = i
					continue
				}

				foundBinary = true
			}
		} else {
			// We are on a parameter
			if token.kind == Field && regexParamDeny.MatchString(token.val) {
				// The parameter needs to be obfuscated

				// Check if the next token is an equal sign and has a value
				if index+1 < len(tokens) && tokens[index+1].kind == Equal {
					// Check if the next token exists (check for end of command)
					if index+2 == len(tokens) {
						// Skip the equal sign, do not obfuscate the value as it doesn't exist
						index++
						continue
					}

					// Get tokens that defines the value
					startValueToken := index + 2
					endValueToken, nbrGrabbed := grabFullArgument(tokens, index+2)

					// Replace the startValueToken with an obfuscated value
					// Only if it expands a value (more than one '=' token)
					if nbrGrabbed > 0 {
						obfuscatedSlices = append(obfuscatedSlices, ObfuscatedSlice{tokens[startValueToken].start, tokens[endValueToken].end})
					}

					// Increment the index to the end of the value
					index = endValueToken
				} else {
					// Replace the next value with an obfuscated value
					if index+1 < len(tokens) {
						// Get tokens that defines the value
						startValueToken := index + 1
						endValueToken, nbrGrabbed := grabFullArgument(tokens, index+1)

						// Replace the startValueToken with an obfuscated value
						if nbrGrabbed > 0 {
							obfuscatedSlices = append(obfuscatedSlices, ObfuscatedSlice{tokens[startValueToken].start, tokens[endValueToken].end})
						}

						// Increment the index to the end of the value
						index = endValueToken
					}
				}
			} else if token.kind == Control || token.kind == Redirection {
				// We found a control or redirection token, we are not on a parameter anymore
				foundBinary = false
			} else if token.kind == Dollar && index+1 < len(tokens) && tokens[index+1].kind == ParentheseOpen {
				// We are on a subcommand $()
				newIndex, obfuscatedSlicesSubcommand := handleSubcommands(tokens, index, index+2, ParentheseOpen)
				index = newIndex
				obfuscatedSlices = append(obfuscatedSlices, obfuscatedSlicesSubcommand...)
			} else if token.kind == Backticks {
				// We are on a subcommand ``
				newIndex, obfuscatedSlicesSubcommand := handleSubcommands(tokens, index, index+1, Backticks)
				if newIndex == index {
					// Failed to find the closing backticks, skip the token
					// Infer that the token next to the backticks would be an executable
					foundBinary = false
				}

				index = newIndex
				obfuscatedSlices = append(obfuscatedSlices, obfuscatedSlicesSubcommand...)
			} else if token.kind == ParentheseOpen {
				// We are on a subcommand ()
				newIndex, obfuscatedSlicesSubcommand := handleSubcommands(tokens, index, index+1, ParentheseOpen)
				if newIndex == index {
					// Failed to find the closing parenthesis, skip the token
					// Infer that the token next to the parenthesis would be an executable
					foundBinary = false
				}

				index = newIndex
				obfuscatedSlices = append(obfuscatedSlices, obfuscatedSlicesSubcommand...)
			}
		}
	}

	return obfuscatedSlices
}

func handleSubcommands(tokens []ShellToken, index int, startIndex int, subcommandType ShellTokenKind) (int, []ObfuscatedSlice) {
	// We are on a variable or a subcommand
	// Get the next token to know if that's an open parenthesis and obfuscate the whole subcommand
	var obfuscatedSlices []ObfuscatedSlice

	openType := ParentheseOpen
	closingType := ParentheseClose

	if subcommandType == Backticks {
		openType = Backticks
		closingType = Backticks
	}

	// Get all tokens from that subcommand and call this function recursively
	nbrOpenParentheses := 1

	// Starting index changes depending on the type of subcommand
	i := startIndex

	for ; i < len(tokens); i++ {
		if tokens[i].kind == closingType {
			nbrOpenParentheses--
		} else if tokens[i].kind == openType {
			nbrOpenParentheses++
		}

		if nbrOpenParentheses == 0 {
			// We found the end of the subcommand
			endSubcommandToken := i

			// Check if at least one token is between the open and close
			if endSubcommandToken == startIndex {
				break
			}

			// Slices of tokens that defines the subcommand
			subcommandTokens := tokens[startIndex:endSubcommandToken] // without the open and close parentheses

			// Call this function recursively
			obfuscatedSlices = obfuscateShellCommandToken(subcommandTokens)

			// Increment the index to the end of the subcommand
			index = endSubcommandToken
			break
		}
	}

	return index, obfuscatedSlices
}

// grabFullArgument grabs the full argument from the given tokens starting at the given index.
// returns the index of the last token of the argument and the number of tokens grabbed.
// For examples:
// - if the argument starting at index 3 is a Dollar token, then it should also grab the next token (calling itself recursively) as part of the argument.
// - if the argument starting at index 3 is a Field token, then it should only grab the Field token.
// - if the argument starting at index 3 is an Equal token, then it should grab the Equal token and the next token (calling itself recursively).
// - if the argument starting at index 3 is a SingleQuote or DoubleQuote token, then it should grab the whole quoted string.
func grabFullArgument(tokens []ShellToken, index int) (int, int) {
	tokensLength := len(tokens)
	if index >= tokensLength {
		return tokensLength - 1, 0 // Return the last token index if we are out of bounds
	}

	nbrGrabbed := 0
	for ; index < tokensLength; index++ {
		kind := tokens[index].kind

		// We can't grab a control or redirection token
		if kind == Control || kind == Redirection {
			return index - 1, nbrGrabbed
		}

		// Grab only the current token
		if kind == Field || kind == Equal || kind == ShellVariable {
			return index, nbrGrabbed + 1
		}

		// Grab the next token
		if kind == Dollar || kind == Equal {
			nbrGrabbed++
			// continue
		} else if kind == ParentheseOpen {
			// Grab the whole parentheses content
			j := index + 1
			for ; j < tokensLength && tokens[j].kind != ParentheseClose; j++ {
				if tokens[j].kind == Dollar {
					// Grab the next token
					nbrGrabbed++
					// continue
				} else if tokens[j].kind == ParentheseOpen {
					// Recursively grab the whole parentheses content
					newIndex, newNbrGrabbed := grabFullArgument(tokens, j)
					nbrGrabbed += newNbrGrabbed
					j = newIndex
				} else {
					nbrGrabbed++
				}
			}

			index = j

			// Add the last token if it is the same as the start token
			if index < tokensLength && tokens[index].kind == ParentheseClose {
				nbrGrabbed++
			}

			break
		} else if kind == SingleQuote || kind == DoubleQuote || kind == Backticks {
			nbrGrabbed++
			index++

			// Grab the whole quoted string
			for ; index < tokensLength && tokens[index].kind != kind; index++ {
				nbrGrabbed++
			}

			// Add the last token if it is the same as the start token
			if index < tokensLength && tokens[index].kind == kind {
				nbrGrabbed++
			}

			break
		} else if kind == Executable {
			nbrGrabbed++
			break
		} else {
			// Not a known token kind
			break
		}
	}

	return index, nbrGrabbed
}

// obfuscateCommandTokens obfuscates the given command arguments as a ShellToken array.
// isExecCmd is true if the command is an exec command and preObfuscatedIndex is the index of the shell command for exec commands
// returns the ShellToken array with obfuscated tokens and the indices of tokens that were obfuscated as a string array
func obfuscateCommandTokens(tokens []ShellToken, isExecCmd bool, preObfuscatedIndex int) []string {
	var obfuscatedArgsIndices []string

	foundBinary := false
	skipNext := false

	for index := 0; index < len(tokens); index++ {
		token := tokens[index]

		if skipNext {
			skipNext = false
			continue
		}

		// Skip the obfuscation of the shell command for exec commands (already obfuscated)
		if index == preObfuscatedIndex {
			continue
		}

		if !foundBinary {
			// Is this an environment variable? Assume that the format match our regex
			if result := regexEnvVars.FindStringSubmatch(token.val); result != nil {
				// If this is an environment variable, check if it’s part of our passlist
				if !stringInSlice(token.val, allowedEnvVars) {
					tokens[index].val = result[1] + "=?"
					obfuscatedArgsIndices = append(obfuscatedArgsIndices, strconv.Itoa(index)+"-"+strconv.Itoa(len(result[1])+1))
				}
			} else {
				// If not formatted like an env variable, likely the binary
				if stringInSlice(token.val, processDenyList) {
					// Remove every parameter until the end of the command
					if isExecCmd {
						// Remove all tokens until the end (the whole command)
						for index++; index < len(tokens); index++ {
							tokens[index].val = "?"
							obfuscatedArgsIndices = append(obfuscatedArgsIndices, strconv.Itoa(index))
						}
					} else {
						for index++; index < len(tokens) && tokens[index].kind == Field; index++ {
							tokens[index].val = "?"
							obfuscatedArgsIndices = append(obfuscatedArgsIndices, strconv.Itoa(index))
						}
					}
				}

				foundBinary = true
			}
		} else { // Alright, we’re in the parameters then
			// Are we in the case of --pass=xxx or --pass xxx
			if equalIndex := strings.Index(token.val, "="); equalIndex == -1 {

				// if --pass xxx, check is --pass is allowed and that we have a xxx
				if regexParamDeny.MatchString(token.val) && index < len(tokens)-1 {
					(tokens)[index+1].val = "?"
					obfuscatedArgsIndices = append(obfuscatedArgsIndices, strconv.Itoa(index+1))
				}

				skipNext = true
			} else {
				// split at the first equal sign
				param := token.val[:equalIndex]

				if regexParamDeny.MatchString(param) {
					tokens[index].val = param + "=?"
					obfuscatedArgsIndices = append(obfuscatedArgsIndices, strconv.Itoa(index)+"-"+strconv.Itoa(len(param)+1))
				}
			}
		}
	}

	return obfuscatedArgsIndices
}

// findShellCommand search for the shell command in the given shell struct of the cmd.exec command
func findShellCommand(commandArgs []string) int {
	// Search for the -c parameter in the command arguments to detect if there is a shell execution
	var cArgDetected bool
	var argMayWaitForArg bool
	for index, arg := range commandArgs {
		// The argument -c can have other arguments inside the same string
		// If it's starting with a dash, it's a parameter, then check if the letter 'c' is inside
		if len(arg) > 0 && arg[0] == '-' {
			// That's a parameter
			argMayWaitForArg = true

			// Ignore double dashed parameters (they can have a value)
			if len(arg) > 1 && arg[1] != '-' && strings.Contains(arg, "c") {
				cArgDetected = true
			}
		} else {
			// That's a command
			if cArgDetected {
				return index + 1 // +1 to skip the -c parameter
			} else {
				if argMayWaitForArg {
					argMayWaitForArg = false
					continue
				}

				// Not an injected shell command via -c
				return -1
			}
		}
	}

	return -1
}

// stringInSlice checks if the given string is present in the given slice
func stringInSlice(str string, list []string) bool {
	for _, item := range list {
		if item == str {
			return true
		}
	}

	return false
}
