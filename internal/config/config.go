// Package config loads and persists the needlectl client configuration.
//
// The configuration lives in ~/.needlectl.conf and uses a minimal
// "key = value" syntax with optional quotes, '#' comments and ';' comments.
// Values may be overridden through environment variables so that the tool can
// also run non-interactively (for example in CI).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// FileName is the configuration file name, resolved inside $HOME.
	FileName = ".needlectl.conf"
	// FileMode keeps credentials readable only by the owner.
	FileMode os.FileMode = 0o600
)

// Environment variable names that override individual configuration values.
const (
	EnvServer             = "NEEDLECTL_SERVER"
	EnvUsername           = "NEEDLECTL_USERNAME"
	EnvPassword           = "NEEDLECTL_PASSWORD"
	EnvInsecureSkipVerify = "NEEDLECTL_INSECURE_SKIP_VERIFY"
)

var (
	// ErrNotFound is returned when the configuration file does not exist.
	ErrNotFound = errors.New("config file not found")
	// ErrIncomplete is returned when required values are missing.
	ErrIncomplete = errors.New("config file is incomplete")
)

// Config holds the connection settings for a DroppedNeedle instance.
type Config struct {
	Server             string
	Username           string
	Password           string
	InsecureSkipVerify bool
}

// Path returns the default configuration file path (~/.needlectl.conf).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, FileName), nil
}

// Load reads the configuration from the default path and applies environment
// overrides.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the configuration from path and applies environment
// overrides. A missing file yields ErrNotFound.
func LoadFrom(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	cfg, err := Parse(file)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyEnv(os.Getenv)
	return cfg, nil
}

// Parse reads a configuration from r. Unknown keys are ignored so that newer
// versions of the file stay backwards compatible.
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{}
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		key, value, ok, err := parseLine(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if !ok {
			continue
		}
		if err := cfg.set(key, value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

// Validate reports the missing fields required to talk to a server.
func (c *Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Server) == "" {
		missing = append(missing, "server")
	}
	if strings.TrimSpace(c.Username) == "" {
		missing = append(missing, "username")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrIncomplete, strings.Join(missing, ", "))
	}
	return nil
}

// String renders the configuration in the on-disk format.
func (c *Config) String() string {
	var b strings.Builder
	b.WriteString("# needlectl configuration file.\n")
	b.WriteString("# Keep this file private: it stores your DroppedNeedle password.\n")
	fmt.Fprintf(&b, "server = %s\n", strconv.Quote(strings.TrimSpace(c.Server)))
	fmt.Fprintf(&b, "username = %s\n", strconv.Quote(strings.TrimSpace(c.Username)))
	fmt.Fprintf(&b, "password = %s\n", strconv.Quote(c.Password))
	fmt.Fprintf(&b, "insecure_skip_verify = %t\n", c.InsecureSkipVerify)
	return b.String()
}

// Save atomically writes the configuration to path with 0600 permissions.
func Save(path string, c *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(FileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := io.WriteString(tmp, c.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

// Permissions returns the file mode of path, or 0 when it does not exist.
func Permissions(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func (c *Config) set(key, value string) error {
	switch key {
	case "server", "url", "base_url":
		c.Server = value
	case "username", "user":
		c.Username = value
	case "password", "pass":
		c.Password = value
	case "insecure_skip_verify", "insecure":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("insecure_skip_verify: expected true or false, got %q", value)
		}
		c.InsecureSkipVerify = parsed
	default:
		// Forward compatible: ignore keys we do not know yet.
	}
	return nil
}

func (c *Config) applyEnv(getenv func(string) string) {
	if v := strings.TrimSpace(getenv(EnvServer)); v != "" {
		c.Server = v
	}
	if v := getenv(EnvUsername); v != "" {
		c.Username = v
	}
	if v := getenv(EnvPassword); v != "" {
		c.Password = v
	}
	if v := strings.TrimSpace(getenv(EnvInsecureSkipVerify)); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			c.InsecureSkipVerify = parsed
		}
	}
}

// parseLine splits a single configuration line into a key and a value.
// Blank lines and comments are reported with ok == false.
func parseLine(raw string) (key, value string, ok bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false, nil
	}

	separator := strings.Index(line, "=")
	if separator < 0 {
		return "", "", false, fmt.Errorf("expected 'key = value', got %q", line)
	}

	key = strings.ToLower(strings.TrimSpace(line[:separator]))
	if key == "" {
		return "", "", false, errors.New("empty key")
	}

	value, err = decodeValue(strings.TrimSpace(line[separator+1:]))
	if err != nil {
		return "", "", false, err
	}
	return key, value, true, nil
}

// decodeValue removes surrounding quotes when present; otherwise it strips a
// trailing " #comment".
func decodeValue(raw string) (string, error) {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid quoted value: %w", err)
		}
		return unquoted, nil
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return raw[1 : len(raw)-1], nil
	}
	if comment := strings.Index(raw, " #"); comment >= 0 {
		raw = strings.TrimSpace(raw[:comment])
	}
	return raw, nil
}
