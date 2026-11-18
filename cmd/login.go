package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mimmo/needlectl/internal/api"
	"github.com/mimmo/needlectl/internal/config"
	"github.com/mimmo/needlectl/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultServer = "http://localhost:8688"

type loginInput struct {
	username   string
	skipVerify bool
}

func newLoginCommand(opts *globalOptions) *cobra.Command {
	input := loginInput{}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store DroppedNeedle connection settings in ~/.needlectl.conf",
		Long: `Prompt for the server URL, username and password, verify them against the
instance and write the result to the configuration file (~/.needlectl.conf).

The file is written with 0600 permissions because it stores the password.

Pass --server and --username to skip the prompts; set NEEDLECTL_PASSWORD when no
terminal is available.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, opts, input)
		},
	}

	cmd.Flags().BoolVar(&input.skipVerify, "skip-verify", false, "save the settings without checking them against the server")
	cmd.Flags().StringVar(&input.username, "username", "", "username (skips that prompt)")

	return cmd
}

func runLogin(cmd *cobra.Command, opts *globalOptions, input loginInput) error {
	printer := opts.printer()
	outputWriter := cmd.OutOrStdout()

	existing, path, err := opts.loadExistingConfig()
	if err != nil {
		return err
	}

	server := firstNonEmpty(opts.serverOverride, os.Getenv(config.EnvServer), existing.Server)
	username := firstNonEmpty(input.username, os.Getenv(config.EnvUsername), existing.Username)
	password := firstNonEmpty(os.Getenv(config.EnvPassword), existing.Password)

	stdinFile, isFile := cmd.InOrStdin().(*os.File)
	interactive := isFile && output.IsTerminal(stdinFile)

	if interactive {
		reader := bufio.NewReader(cmd.InOrStdin())
		if strings.TrimSpace(opts.serverOverride) == "" {
			if server, err = promptLine(outputWriter, reader, "Server URL", firstNonEmpty(server, defaultServer)); err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.username) == "" {
			if username, err = promptLine(outputWriter, reader, "Username", username); err != nil {
				return err
			}
		}
		if password, err = promptPassword(outputWriter, stdinFile, password); err != nil {
			return err
		}
	}

	if strings.TrimSpace(server) == "" || strings.TrimSpace(username) == "" || password == "" {
		return fmt.Errorf("missing credentials; provide --server/--username and set %s when no terminal is available", config.EnvPassword)
	}

	cfg := &config.Config{
		Server:   server,
		Username: username,
		Password: password,
	}
	opts.applyOverrides(cfg)

	if !input.skipVerify {
		printer.Step("Checking credentials against %s", strings.TrimRight(cfg.Server, "/"))
		client, err := api.New(cfg.Server, cfg.InsecureSkipVerify)
		if err != nil {
			return err
		}
		user, err := client.Login(cmd.Context(), cfg.Username, cfg.Password)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		printer.Success("Signed in as %s (%s)", user.DisplayName, user.Role)
	}

	warnIfLoosePermissions(printer, path)
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	printer.Success("Saved configuration to %s", path)
	return nil
}

func promptLine(out io.Writer, reader *bufio.Reader, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}

	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func promptPassword(out io.Writer, stdin *os.File, current string) (string, error) {
	if current != "" {
		fmt.Fprint(out, "Password [keep current]: ")
	} else {
		fmt.Fprint(out, "Password: ")
	}

	raw, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	value := strings.TrimSpace(string(raw))
	if value == "" {
		return current, nil
	}
	return value, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
