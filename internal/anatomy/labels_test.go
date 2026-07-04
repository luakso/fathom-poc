package anatomy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func writeLabels(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "entity-labels.json")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadManualLabels_OK(t *testing.T) {
	p := writeLabels(t, `{
		"labels": [
			{"chain": "base", "address": "0xA9DD77c96f2C68F7502cbCBE7f0b8Ec54D072315",
			 "label": "api.tollbit.com", "url": "https://tollbit.com", "note": "content licensing API"}
		]
	}`)
	labels, err := anatomy.LoadManualLabels(p)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	// EVM addresses are normalized to lowercase.
	require.Equal(t, "0xa9dd77c96f2c68f7502cbcbe7f0b8ec54d072315", labels[0].Address)
	require.Equal(t, "api.tollbit.com", labels[0].Label)
	require.Equal(t, "base", labels[0].Chain)
}

func TestLoadManualLabels_Rejects(t *testing.T) {
	cases := map[string]string{
		"unknown chain":  `{"labels":[{"chain":"solana","address":"0x1234567890123456789012345678901234567890","label":"x"}]}`,
		"bad address":    `{"labels":[{"chain":"base","address":"0xnothex","label":"x"}]}`,
		"empty label":    `{"labels":[{"chain":"base","address":"0x1234567890123456789012345678901234567890","label":""}]}`,
		"duplicate":      `{"labels":[{"chain":"base","address":"0x1234567890123456789012345678901234567890","label":"a"},{"chain":"base","address":"0x1234567890123456789012345678901234567890","label":"b"}]}`,
		"malformed json": `{"labels":`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := anatomy.LoadManualLabels(writeLabels(t, content))
			require.Error(t, err)
		})
	}
}

func TestLoadManualLabels_MissingFile(t *testing.T) {
	_, err := anatomy.LoadManualLabels(filepath.Join(t.TempDir(), "absent.json"))
	require.Error(t, err)
}

func TestLoadManualLabels_CommittedFile(t *testing.T) {
	labels, err := anatomy.LoadManualLabels("../../data/entity-labels.json")
	require.NoError(t, err)
	// The committed file may legitimately be empty; the gate is that it parses.
	require.NotNil(t, labels)
}
