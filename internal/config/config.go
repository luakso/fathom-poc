// Package config loads layered TOML config for fathom binaries.
//
// Layers, each overriding the previous:
//  1. config/<binary>/base.toml
//  2. config/<binary>/<env>.toml
//  3. environment variables (FOO__BAR -> foo.bar)
//  4. file at BasicConfig.SecretsPath, if it exists
//
// Generic over a per-binary Config type that embeds BasicConfig.
package config

import (
	"fmt"
	"os"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// BasicConfigurator is the contract every per-binary Config struct must satisfy
// so ParseConfig can validate the shared mandatory fields.
type BasicConfigurator interface {
	GetBasicConfig() BasicConfig
}

// BasicConfig holds fields every binary needs. Per-binary Config structs embed
// this and add their own sections.
type BasicConfig struct {
	Env     string `koanf:"env"`
	Name    string `koanf:"name"`
	Version string `koanf:"version"`

	SecretsPath string `koanf:"secrets_path"`

	Log struct {
		IsPretty bool   `koanf:"is_pretty"`
		Level    string `koanf:"level"`
	} `koanf:"log"`

	WithDebugProfiler bool `koanf:"with_debug_profiler"`
}

// ParseConfig loads the four layers and unmarshals into Config. The binaryName
// determines the config directory (config/<binaryName>/), and environment
// selects which env-specific overlay (config/<binaryName>/<environment>.toml).
func ParseConfig[Config BasicConfigurator](binaryName, environment string) (Config, error) {
	var empty Config
	k := koanf.New(".")

	// Layer 1: base.toml — required.
	basePath := fmt.Sprintf("config/%s/base.toml", binaryName)
	if err := k.Load(file.Provider(basePath), toml.Parser()); err != nil {
		return empty, fmt.Errorf("load base config (%s, %s): %w", binaryName, environment, err)
	}

	// Layer 2: <env>.toml — required.
	envPath := fmt.Sprintf("config/%s/%s.toml", binaryName, environment)
	if err := k.Load(file.Provider(envPath), toml.Parser()); err != nil {
		return empty, fmt.Errorf("load env config (%s, %s): %w", binaryName, environment, err)
	}

	// Layer 3: environment variables. FOO__BAR -> foo.bar using "__" as delimiter.
	// The v2 env.Provider uses env.Provider(delim, Opt) where Opt.TransformFunc
	// receives (key, value) and returns (transformedKey, transformedValue).
	envProvider := env.Provider(".", env.Opt{
		TransformFunc: func(k, v string) (string, interface{}) {
			return strings.ReplaceAll(strings.ToLower(k), "__", "."), v
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return empty, fmt.Errorf("load env vars (%s, %s): %w", binaryName, environment, err)
	}

	if val, ok := k.Get("secrets_path").(string); ok && val != "" {
		if _, err := os.Stat(val); err == nil {
			if err := k.Load(file.Provider(val), toml.Parser()); err != nil {
				return empty, fmt.Errorf("load secrets %s: %w", val, err)
			}
		}
	}

	var cfg Config
	err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
			// WeaklyTypedInput matches koanf's bare Unmarshal default,
			// allowing string→number coercions from env var overrides.
			WeaklyTypedInput: true,
			// Squash enables embedded struct field promotion so that
			// fields in embedded structs (like BasicConfig) are mapped
			// from top-level koanf keys rather than a nested sub-map.
			Squash: true,
			Result: &cfg,
		},
	})
	if err != nil {
		return empty, fmt.Errorf("unmarshal config (%s, %s): %w", binaryName, environment, err)
	}

	b := cfg.GetBasicConfig()
	if b.Name == "" || b.Version == "" || b.Env == "" {
		return empty, MissingRequiredFieldsError{
			BinaryName: binaryName, Env: b.Env, Name: b.Name, Version: b.Version,
		}
	}

	return cfg, nil
}
