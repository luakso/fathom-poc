package config

import "fmt"

// MissingRequiredFieldsError is returned by ParseConfig when one of the
// mandatory BasicConfig fields (Env, Name, Version) is empty after all four
// layers have been merged.
type MissingRequiredFieldsError struct {
	BinaryName  string
	Environment string // the APP_ENV argument passed to ParseConfig
	Env         string // the parsed env field from TOML (empty when missing)
	Name        string
	Version     string
}

func (m MissingRequiredFieldsError) Error() string {
	return fmt.Sprintf(
		"missing required fields in config(%s, env=%q): name=%q version=%q parsed_env=%q",
		m.BinaryName, m.Environment, m.Name, m.Version, m.Env,
	)
}
