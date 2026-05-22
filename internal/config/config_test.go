package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lukostrobl/fathom/internal/config"
)

type testCfg struct {
	config.BasicConfig
	Database struct {
		URL string `koanf:"url"`
	} `koanf:"database"`
}

func (c testCfg) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

// writeCfg writes a config tree rooted at dir/config/<binary>/ and returns dir.
// chdir into dir before calling ParseConfig so the relative paths in the loader resolve.
func writeCfg(t *testing.T, binary, base, env, secrets string) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "config", binary)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if base != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "base.toml"), []byte(base), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "local.toml"), []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if secrets != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "local.secrets.toml"), []byte(secrets), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestParseConfig_LoadsBaseAndEnv(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" + `[database]` + "\n" + `url = "postgres://x"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Name != "x" || cfg.Version != "v0" || cfg.Env != "local" {
		t.Errorf("BasicConfig fields wrong: %+v", cfg.BasicConfig)
	}
	if cfg.Database.URL != "postgres://x" {
		t.Errorf("Database.URL = %q, want postgres://x", cfg.Database.URL)
	}
}

func TestParseConfig_EnvVarOverridesTOML(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n" + `[log]` + "\n" + `level = "info"` + "\n"
	env := `env = "local"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	t.Setenv("LOG__LEVEL", "warn")

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want warn (env override)", cfg.Log.Level)
	}
}

func TestParseConfig_SecretsFileOverlays(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" +
		`secrets_path = "config/x/local.secrets.toml"` + "\n" +
		`[database]` + "\n" +
		`url = "postgres://placeholder"` + "\n"
	secrets := `[database]` + "\n" + `url = "postgres://real"` + "\n"
	dir := writeCfg(t, "x", base, env, secrets)
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Database.URL != "postgres://real" {
		t.Errorf("Database.URL = %q, want postgres://real", cfg.Database.URL)
	}
}

func TestParseConfig_SecretsFileMissingIsSilent(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" + `secrets_path = "config/x/local.secrets.toml"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v (secrets file absent must be silent)", err)
	}
	if cfg.Name != "x" {
		t.Errorf("BasicConfig.Name = %q", cfg.Name)
	}
}

func TestParseConfig_MissingRequiredFields(t *testing.T) {
	// base.toml has no name/version; <env>.toml only sets env.
	base := ""
	env := `env = "local"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	_, err := config.ParseConfig[testCfg]("x", "local")
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	var missing config.MissingRequiredFieldsError
	if !errors.As(err, &missing) {
		t.Errorf("error type = %T, want MissingRequiredFieldsError", err)
	}
}

func TestParseConfig_MissingBaseFile(t *testing.T) {
	// no base.toml, no env.toml
	dir := t.TempDir()
	chdir(t, dir)
	_, err := config.ParseConfig[testCfg]("x", "local")
	if err == nil {
		t.Fatal("expected error when base.toml is missing")
	}
}
