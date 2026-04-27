package fileui

import (
	"context"
	"fmt"
	"strings"

	"github.com/cmdblock/cbssh/internal/filetransfer"
)

func (u *ui) runRemoteCommand(ctx context.Context, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		command = u.prompt("Remote command", "")
	}
	if strings.TrimSpace(command) == "" {
		return errCanceled
	}

	output, err := u.session.RunCommand(ctx, remoteShellCommand(u.cwd, command))
	printRemoteCommandResult(command, output, err)
	u.waitEnter()

	if refreshErr := u.refresh(); refreshErr != nil {
		return refreshErr
	}
	return nil
}

func printRemoteCommandResult(command string, output filetransfer.CommandOutput, err error) {
	fmt.Println()
	fmt.Printf("%s$%s %s\n", styleBold, styleReset, command)
	if output.Stdout != "" {
		printCapturedOutput(output.Stdout)
	}
	if output.Stderr != "" {
		if output.Stdout != "" {
			fmt.Println()
		}
		printCapturedOutput(output.Stderr)
	}
	if err != nil {
		fmt.Printf("%sRemote command failed: %v%s\n", styleRed, err, styleReset)
		return
	}
	if output.Stdout == "" && output.Stderr == "" {
		fmt.Println(styleGreen + "Remote command completed with no output." + styleReset)
	}
}

func printCapturedOutput(value string) {
	fmt.Print(value)
	if !strings.HasSuffix(value, "\n") {
		fmt.Println()
	}
}

func remoteShellCommand(cwd string, command string) string {
	return "cd " + shellQuote(cwd) + " && " + command
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
