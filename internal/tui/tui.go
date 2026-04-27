package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/hostview"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

var errCanceled = errors.New("canceled")

var lastError string

const (
	styleBold  = "\033[1m"
	styleGreen = "\033[32m"
	styleRed   = "\033[31m"
	styleCyan  = "\033[36m"
	styleDim   = "\033[2m"
	styleReset = "\033[0m"
)

type menuCommand struct {
	Action string
	Args   []string
}

// Run opens the interactive management loop and reloads config on each screen.
func Run(ctx context.Context, configPath string, statePath string) error {
	reader := bufio.NewReader(os.Stdin)
	defer func() { stdinClosed = true }()
	var lastCfg model.Config
	sortRecent := true
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			if lastCfg.Hosts == nil {
				return err
			}
			cfg = lastCfg
			fmt.Printf("%sWarning: config load failed, using cached: %v%s\n", styleRed, err, styleReset)
		} else {
			lastCfg = cfg
		}
		st, _, err := tunnel.Status(statePath, "")
		if err != nil {
			fmt.Printf("%sWarning: tunnel status error: %v%s\n", styleRed, err, styleReset)
		}
		sortMode := hostview.SortName
		if sortRecent {
			sortMode = hostview.SortRecent
		}
		sorted, err := hostview.Sort(cfg.Hosts, st, sortMode)
		if err != nil {
			return err
		}
		setSortedHosts(sorted)
		clearScreen()
		printDashboard(configPath, sorted, cfg, st, sortRecent)
		rawInput, readErr := readChoice(reader, "Action")
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		if stdinClosed {
			return nil
		}
		choice := parseCommand(rawInput)
		switch choice.Action {
		case "":
			continue
		case "?":
			printMainHelp()
			waitEnter(reader)
			continue
		case "q", "b":
			return nil
		case "a":
			err = addHost(reader, configPath, cfg)
		case "e":
			err = editHost(reader, configPath, cfg, firstArg(choice.Args))
		case "d":
			err = deleteHost(reader, configPath, cfg, firstArg(choice.Args))
		case "t":
			err = manageTunnels(ctx, reader, configPath, statePath, cfg, firstArg(choice.Args))
		case "i":
			err = showHost(reader, statePath, cfg, firstArg(choice.Args))
		case "s":
			err = startTunnels(ctx, reader, statePath, configPath, cfg, choice.Args)
		case "x":
			err = stopTunnels(ctx, reader, statePath, cfg, choice.Args)
		case "c":
			err = connectHost(ctx, reader, configPath, statePath, cfg, firstArg(choice.Args))
		case "f":
			err = browseFiles(ctx, reader, statePath, cfg, firstArg(choice.Args))
		case "v":
			err = config.Validate(cfg)
		case "r":
			sortRecent = !sortRecent
			continue
		default:
			err = fmt.Errorf("unknown command %q, press ? for help", choice.Action)
		}
		if err != nil {
			if errors.Is(err, errCanceled) {
				continue
			}
			lastError = err.Error()
		}
	}
}
