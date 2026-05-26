// Package internal provides internal utilities for the fathom service.
package internal

import (
	_ "github.com/ethereum/go-ethereum/crypto" // Used for keccak256 hashing
	_ "github.com/shopspring/decimal"          // Used for NUMERIC(78,0) database roundtrips with pgx
	_ "github.com/stretchr/testify/require"    // Used for test assertions
)
