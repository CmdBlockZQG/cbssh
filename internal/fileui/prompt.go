package fileui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (u *ui) readLine(label string) (string, error) {
	fmt.Printf("%s: ", label)
	return readLine(u.reader)
}

func (u *ui) prompt(label string, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	value, _ := readLine(u.reader)
	if value == "" {
		return fallback
	}
	return value
}

func (u *ui) confirm(label string, fallback bool) bool {
	defaultValue := "n"
	if fallback {
		defaultValue = "y"
	}
	for {
		value := strings.ToLower(u.prompt(label+" (y/n)", defaultValue))
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

func (u *ui) waitEnter() {
	fmt.Print("Press Enter to continue...")
	_, _ = u.reader.ReadString('\n')
}

func readLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err == io.EOF && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	return strings.TrimSpace(value), err
}

func parseCommand(value string) command {
	value = strings.TrimSpace(value)
	if value == "" {
		return command{}
	}

	if strings.HasPrefix(value, "/") {
		return rawArgCommand("/", strings.TrimSpace(value[1:]))
	}

	rawFields := strings.Fields(value)
	action := strings.ToLower(rawFields[0])
	if action == "x" {
		return rawArgCommand(action, strings.TrimSpace(value[len(rawFields[0]):]))
	}

	fields := strings.Fields(strings.ReplaceAll(value, ",", " "))
	return command{
		action: strings.ToLower(fields[0]),
		args:   fields[1:],
	}
}

func rawArgCommand(action string, arg string) command {
	if arg == "" {
		return command{action: action}
	}
	return command{action: action, args: []string{arg}}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func parseEntryNumber(value string) (int, bool) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	return number, err == nil
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
