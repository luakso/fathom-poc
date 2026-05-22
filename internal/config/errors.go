package config

import "fmt"

// MissingRequiredFieldsError is returned by ParseConfig when one of the
// mandatory BasicConfig fields (Env, Name, Version) is empty after all four
// layers have been merged.
type MissingRequiredFieldsError struct {
	BinaryName string
	Env        string
	Name       string
	Version    string
}

func (m MissingRequiredFieldsError) Error() string {
	return fmt.Sprintf(
		"missing required fields in config(%s, %s): name=%q version=%q",
		m.BinaryName, m.Env, m.Name, m.Version,
	)
}
