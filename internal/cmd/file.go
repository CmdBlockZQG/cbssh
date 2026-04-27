package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/filetransfer"
	"github.com/cmdblock/cbssh/internal/fileui"
	"github.com/cmdblock/cbssh/internal/state"
)

func (a *app) newFileCommand() *cobra.Command {
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Transfer files over SFTP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	fileCmd.AddCommand(a.newFileUploadCommand())
	fileCmd.AddCommand(a.newFileDownloadCommand())
	fileCmd.AddCommand(a.newFileTUICommand())
	return fileCmd
}

func (a *app) newFileTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui <name>",
		Aliases: []string{"browse"},
		Short:   "Browse remote files over SFTP",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if err := fileui.Run(cmd.Context(), cfg, args[0]); err != nil {
				return err
			}
			return nil
		},
	}
}

func (a *app) newBrowseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "browse <name>",
		Short: "Alias for 'file tui'",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if err := fileui.Run(cmd.Context(), cfg, args[0]); err != nil {
				return err
			}
			return nil
		},
	}
}

// Top-level up/down/browse commands stay as shortcuts, while file upload/file
// download/file tui remain the canonical command namespace.
func (a *app) newFileUploadCommand() *cobra.Command {
	var opts filetransfer.Options
	var quiet bool
	c := &cobra.Command{
		Use:     "upload <name> <local> [remote]",
		Aliases: []string{"up"},
		Short:   "Upload files over SFTP",
		Long:    remotePathHelp("Upload files over SFTP"),
		Args:    cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			remotePath := ""
			if len(args) == 3 {
				remotePath = args[2]
			}
			result, err := filetransfer.Upload(cmd.Context(), cfg, args[0], args[1], remotePath, opts)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if !quiet {
				printTransferResult(cmd.OutOrStdout(), "Uploaded", result)
			}
			return nil
		},
	}
	addFileTransferFlags(c, &opts, &quiet)
	return c
}

func (a *app) newFileDownloadCommand() *cobra.Command {
	var opts filetransfer.Options
	var quiet bool
	c := &cobra.Command{
		Use:     "download <name> <remote> [local]",
		Aliases: []string{"down"},
		Short:   "Download files over SFTP",
		Long:    remotePathHelp("Download files over SFTP"),
		Args:    cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			localPath := ""
			if len(args) == 3 {
				localPath = args[2]
			}
			result, err := filetransfer.Download(cmd.Context(), cfg, args[0], args[1], localPath, opts)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if !quiet {
				printTransferResult(cmd.OutOrStdout(), "Downloaded", result)
			}
			return nil
		},
	}
	addFileTransferFlags(c, &opts, &quiet)
	return c
}

func (a *app) newUpCommand() *cobra.Command {
	var opts filetransfer.Options
	var quiet bool
	c := &cobra.Command{
		Use:   "up <name> <local> [remote]",
		Short: "Alias for 'file upload'",
		Long:  remotePathHelp("Upload files over SFTP"),
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			remotePath := ""
			if len(args) == 3 {
				remotePath = args[2]
			}
			result, err := filetransfer.Upload(cmd.Context(), cfg, args[0], args[1], remotePath, opts)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if !quiet {
				printTransferResult(cmd.OutOrStdout(), "Uploaded", result)
			}
			return nil
		},
	}
	addFileTransferFlags(c, &opts, &quiet)
	return c
}

func (a *app) newDownCommand() *cobra.Command {
	var opts filetransfer.Options
	var quiet bool
	c := &cobra.Command{
		Use:   "down <name> <remote> [local]",
		Short: "Alias for 'file download'",
		Long:  remotePathHelp("Download files over SFTP"),
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			localPath := ""
			if len(args) == 3 {
				localPath = args[2]
			}
			result, err := filetransfer.Download(cmd.Context(), cfg, args[0], args[1], localPath, opts)
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			if !quiet {
				printTransferResult(cmd.OutOrStdout(), "Downloaded", result)
			}
			return nil
		},
	}
	addFileTransferFlags(c, &opts, &quiet)
	return c
}

func addFileTransferFlags(cmd *cobra.Command, opts *filetransfer.Options, quiet *bool) {
	cmd.Flags().BoolVarP(&opts.Recursive, "recursive", "r", false, "Transfer directories recursively")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Overwrite existing files")
	cmd.Flags().BoolVarP(quiet, "quiet", "q", false, "Only print errors")
}

func printTransferResult(out io.Writer, verb string, result filetransfer.Result) {
	switch verb {
	case "Uploaded":
		fmt.Fprintf(out, "%s %s (%s) from %s to %s:%s\n",
			verb,
			transferCount(result),
			formatBytes(result.Bytes),
			result.LocalPath,
			result.HostName,
			result.RemotePath,
		)
	case "Downloaded":
		fmt.Fprintf(out, "%s %s (%s) from %s:%s to %s\n",
			verb,
			transferCount(result),
			formatBytes(result.Bytes),
			result.HostName,
			result.RemotePath,
			result.LocalPath,
		)
	default:
		fmt.Fprintf(out, "%s %s (%s)\n", verb, transferCount(result), formatBytes(result.Bytes))
	}
}

func transferCount(result filetransfer.Result) string {
	parts := []string{plural(result.Files, "file")}
	if result.Directories > 0 {
		parts = append(parts, plural(result.Directories, "directory"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/1024)
}

func remotePathHelp(summary string) string {
	return summary + "\n\nRemote paths may be absolute, relative to the remote initial directory, or use quoted ~/path. Quote or escape remote paths that start with ~, for example '~/app.log' or \\~/app.log, so your local shell does not expand them before cbssh starts."
}
