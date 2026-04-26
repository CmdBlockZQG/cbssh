package tui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cmdblock/cbssh/internal/model"
)

func promptString(reader *bufio.Reader, label string, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	value, _ := readLine(reader)
	if value == "" {
		return fallback
	}
	return value
}

func promptRequiredString(reader *bufio.Reader, label string, fallback string) string {
	for {
		if stdinClosed {
			return ""
		}
		value := promptString(reader, label, fallback)
		if value != "" {
			return value
		}
		fmt.Printf("%s is required.\n", label)
	}
}

func promptPort(reader *bufio.Reader, label string, fallback int) int {
	for {
		if stdinClosed {
			return 0
		}
		var value string
		if fallback > 0 {
			value = promptString(reader, label, strconv.Itoa(fallback))
		} else {
			value = promptString(reader, label, "")
		}
		if value == "" {
			fmt.Println("Please enter a port.")
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil {
			fmt.Println("Please enter a number.")
			continue
		}
		if number < 1 || number > 65535 {
			fmt.Println("Port must be between 1 and 65535.")
			continue
		}
		return number
	}
}

func promptAuthType(reader *bufio.Reader, fallback string) string {
	for {
		if stdinClosed {
			return model.AuthTypeKey
		}
		value := strings.ToLower(promptString(reader, "Auth type (key/password)", fallback))
		switch value {
		case "k", "key":
			return model.AuthTypeKey
		case "p", "pass", "password":
			return model.AuthTypePassword
		default:
			fmt.Println("Please enter key or password.")
		}
	}
}

func promptTunnelType(reader *bufio.Reader, fallback string) string {
	for {
		if stdinClosed {
			return model.TunnelTypeLocal
		}
		value := strings.ToLower(promptString(reader, "Type (local/remote/dynamic)", fallback))
		switch value {
		case "l", "local":
			return model.TunnelTypeLocal
		case "r", "remote":
			return model.TunnelTypeRemote
		case "d", "dynamic":
			return model.TunnelTypeDynamic
		default:
			fmt.Println("Please enter local, remote, or dynamic.")
		}
	}
}

func promptBool(reader *bufio.Reader, label string, fallback bool) bool {
	defaultValue := "n"
	if fallback {
		defaultValue = "y"
	}
	for {
		if stdinClosed {
			return fallback
		}
		value := strings.ToLower(promptString(reader, label+" (y/n)", defaultValue))
		switch value {
		case "y", "yes", "true":
			return true
		case "n", "no", "false":
			return false
		default:
			fmt.Println("Please enter y or n.")
		}
	}
}

var stdinClosed bool

func readChoice(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	return readLine(reader)
}

func readLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err == io.EOF {
		if strings.TrimSpace(value) == "" {
			stdinClosed = true
		}
		return strings.TrimSpace(value), nil
	}
	return strings.TrimSpace(value), err
}

func waitEnter(reader *bufio.Reader) {
	fmt.Print("Press Enter to continue...")
	_, _ = reader.ReadString('\n')
}

func splitArgs(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	return strings.Fields(value)
}

func parseCommand(value string) menuCommand {
	fields := splitArgs(value)
	if len(fields) == 0 {
		return menuCommand{}
	}
	return menuCommand{
		Action: strings.ToLower(fields[0]),
		Args:   fields[1:],
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
