// Package cmd implements the needlectl command line interface.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mimmo/needlectl/internal/config"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
)

// version is overridden at build time with
// -ldflags "-X github.com/mimmo/needlectl/cmd.version=<version>".
var version = "dev"

// globalOptions holds the flags shared by every subcommand.
type globalOptions struct {
	configPath     string
	verbose        bool
	noColor        bool
	serverOverride string
	insecure       bool
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "needlectl: %v\n", err)
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	opts := &globalOptions{}

	root := &cobra.Command{
		Use:   "needlectl",
		Short: "Manage a DroppedNeedle instance from the command line",
		Long: `needlectl talks to a self-hosted DroppedNeedle instance over its REST API.

The connection settings (server URL, username and password) are stored in
~/.needlectl.conf. Run "needlectl login" to create or update that file.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file to use (default ~/.needlectl.conf)")
	root.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "print every decision while running")
	root.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable colored output")
	root.PersistentFlags().StringVar(&opts.serverOverride, "server", "", "override the server URL from the config")
	root.PersistentFlags().BoolVar(&opts.insecure, "insecure", false, "skip TLS certificate verification")

	root.AddCommand(newLoginCommand(opts))
	root.AddCommand(newImportCommand(opts))
	root.AddCommand(newAlbumCommand(opts))
	root.AddCommand(newTrackCommand(opts))

	return root
}

func (o *globalOptions) printer() *output.Printer {
	return output.New(os.Stdout, os.Stderr, output.Options{
		Color:   output.ColorsEnabled(os.Stdout, o.noColor),
		Verbose: o.verbose,
	})
}

// resolvedConfigPath returns the effective configuration file path.
func (o *globalOptions) resolvedConfigPath() (string, error) {
	if strings.TrimSpace(o.configPath) != "" {
		return o.configPath, nil
	}
	return config.Path()
}

// loadExistingConfig reads the config file, tolerating a missing file by
// returning an empty configuration.
func (o *globalOptions) loadExistingConfig() (*config.Config, string, error) {
	path, err := o.resolvedConfigPath()
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return &config.Config{}, path, nil
		}
		return nil, path, err
	}
	return cfg, path, nil
}

// loadConfig reads the config file and requires it to be complete.
func (o *globalOptions) loadConfig(printer *output.Printer) (*config.Config, error) {
	cfg, path, err := o.loadExistingConfig()
	if err != nil {
		return nil, err
	}
	o.applyOverrides(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w\nrun 'needlectl login' to update %s", err, path)
	}
	warnIfLoosePermissions(printer, path)
	return cfg, nil
}

// warnIfLoosePermissions tells the user when the config file (which stores the
// password) is readable by anyone other than its owner.
func warnIfLoosePermissions(printer *output.Printer, path string) {
	mode, err := config.Permissions(path)
	if err != nil || mode == 0 {
		return
	}
	if mode.Perm() != config.FileMode {
		printer.Warn("%s has permissions %#o; run 'needlectl login' to rewrite it as %#o", path, mode.Perm(), config.FileMode)
	}
}

func (o *globalOptions) applyOverrides(cfg *config.Config) {
	if strings.TrimSpace(o.serverOverride) != "" {
		cfg.Server = o.serverOverride
	}
	if o.insecure {
		cfg.InsecureSkipVerify = true
	}
}
