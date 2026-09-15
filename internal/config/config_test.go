package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReadsKnownKeysAndSkipsComments(t *testing.T) {
	input := `
# a comment
; another comment
server = "http://localhost:8688"
username = admin
password = 'p@ss #word'
insecure_skip_verify = true
unknown_key = ignored
`
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Server != "http://localhost:8688" {
		t.Errorf("Server = %q, want %q", cfg.Server, "http://localhost:8688")
	}
	if cfg.Username != "admin" {
		t.Errorf("Username = %q, want %q", cfg.Username, "admin")
	}
	if cfg.Password != "p@ss #word" {
		t.Errorf("Password = %q, want %q", cfg.Password, "p@ss #word")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestParseStripsInlineCommentsFromUnquotedValues(t *testing.T) {
	cfg, err := Parse(strings.NewReader("password = secret # trailing\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Password != "secret" {
		t.Errorf("Password = %q, want %q", cfg.Password, "secret")
	}
}

func TestParseRejectsInvalidBoolean(t *testing.T) {
	_, err := Parse(strings.NewReader("insecure_skip_verify = maybe\n"))
	if err == nil {
		t.Fatal("Parse returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "insecure_skip_verify") {
		t.Errorf("error %q does not mention the offending key", err)
	}
}

func TestParseRejectsLineWithoutSeparator(t *testing.T) {
	_, err := Parse(strings.NewReader("this is not a config line\n"))
	if err == nil {
		t.Fatal("Parse returned nil error, want a failure")
	}
}

func TestValidateReportsMissingFields(t *testing.T) {
	cfg := &Config{Server: "http://localhost:8688"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate returned nil error, want a failure")
	}
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("error = %v, want it to wrap ErrIncomplete", err)
	}
	for _, field := range []string{"username", "password"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error %q does not mention %q", err, field)
		}
	}
}

func TestValidateAcceptsCompleteConfig(t *testing.T) {
	cfg := &Config{Server: "http://localhost:8688", Username: "admin", Password: "secret"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestStringRoundTripsThroughParse(t *testing.T) {
	original := &Config{
		Server:             "https://music.example.test",
		Username:           "admin",
		Password:           `quotes " and = signs`,
		InsecureSkipVerify: true,
	}

	parsed, err := Parse(strings.NewReader(original.String()))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if *parsed != *original {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", parsed, original)
	}
}

func TestSaveWritesPrivateFileAndLoadReadsItBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	want := &Config{Server: "http://localhost:8688", Username: "admin", Password: "secret"}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	mode, err := Permissions(path)
	if err != nil {
		t.Fatalf("Permissions returned error: %v", err)
	}
	if mode.Perm() != FileMode {
		t.Errorf("file mode = %#o, want %#o", mode.Perm(), FileMode)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if *got != *want {
		t.Errorf("loaded config = %+v, want %+v", got, want)
	}
}

func TestLoadFromReturnsErrNotFound(t *testing.T) {
	_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.conf"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestApplyEnvOverridesFileValues(t *testing.T) {
	cfg := &Config{Server: "http://file", Username: "file-user", Password: "file-pass"}
	env := map[string]string{
		EnvServer:             "http://env",
		EnvUsername:           "env-user",
		EnvPassword:           "env-pass",
		EnvInsecureSkipVerify: "true",
	}

	cfg.applyEnv(func(key string) string { return env[key] })

	if cfg.Server != "http://env" || cfg.Username != "env-user" || cfg.Password != "env-pass" {
		t.Errorf("env overrides were not applied: %+v", cfg)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestPathUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path returned error: %v", err)
	}
	if want := filepath.Join(home, FileName); path != want {
		t.Errorf("Path = %q, want %q", path, want)
	}
}

func TestPermissionsOfMissingFileIsZero(t *testing.T) {
	mode, err := Permissions(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("Permissions returned error: %v", err)
	}
	if mode != 0 {
		t.Errorf("mode = %v, want 0", mode)
	}
}

func TestDecodeValueQuotesPreserveHash(t *testing.T) {
	cases := map[string]string{
		`"a # b"`:         "a # b",
		`'a # b'`:         "a # b",
		`plain # comment`: "plain",
		`plain`:           "plain",
	}
	for input, want := range cases {
		got, err := decodeValue(input)
		if err != nil {
			t.Fatalf("decodeValue(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Errorf("decodeValue(%q) = %q, want %q", input, got, want)
		}
	}
}
