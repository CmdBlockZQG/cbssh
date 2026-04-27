package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/platform"
)

func (a *app) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage cbssh configuration",
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), platform.ExpandPath(a.configPath))
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create an empty config file if it does not exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Ensure(a.configPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config ready at %s\n", platform.ExpandPath(a.configPath))
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := config.Load(a.configPath); err != nil {
				return err
			}
			for _, warning := range config.ValidateFilePermissions(a.configPath) {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", warning)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Config is valid.")
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Ensure(a.configPath); err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.CommandContext(cmd.Context(), editor, platform.ExpandPath(a.configPath))
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			return editorCmd.Run()
		},
	})
	return configCmd
}
