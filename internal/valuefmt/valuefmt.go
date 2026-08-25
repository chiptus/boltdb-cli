// Package valuefmt owns the vocabulary for representing raw bbolt bytes as
// CLI-facing text: text, base64, hex, or uint64-be. It is the single place
// that encodes, decodes, and validates that vocabulary — callers that need
// to name a format (guessing one from a bucket's keys, parsing a --format
// flag, rendering a value for display) all go through this package instead
// of each carrying their own copy of the format switch.
package valuefmt

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
)

// Format names how raw bytes are represented as text. The zero value is not
// a valid format on its own — it's used by callers (e.g. GuessKeyFormat) as
// a sentinel for "no guess available," distinct from an explicit Text.
type Format string

const (
	Text     Format = "text"
	Base64   Format = "base64"
	Hex      Format = "hex"
	Uint64BE Format = "uint64-be"
)

// Parse validates s as one of the known formats. An empty string means Text.
func Parse(s string) (Format, error) {
	switch f := Format(s); f {
	case "", Text:
		return Text, nil
	case Base64, Hex, Uint64BE:
		return f, nil
	default:
		return "", fmt.Errorf("unknown format %q (want text, base64, hex, or uint64-be)", s)
	}
}

// Encode renders b as a string in format f.
func Encode(b []byte, f Format) (string, error) {
	switch f {
	case "", Text:
		return string(b), nil
	case Base64:
		return base64.StdEncoding.EncodeToString(b), nil
	case Hex:
		return hex.EncodeToString(b), nil
	case Uint64BE:
		if len(b) != 8 {
			return "", fmt.Errorf("uint64-be format: value is %d bytes, want 8", len(b))
		}
		return strconv.FormatUint(binary.BigEndian.Uint64(b), 10), nil
	default:
		return "", fmt.Errorf("unknown format %q (want text, base64, hex, or uint64-be)", f)
	}
}

// Decode parses s according to format f into raw bytes, the inverse of Encode.
func Decode(s string, f Format) ([]byte, error) {
	switch f {
	case "", Text:
		return []byte(s), nil
	case Base64:
		return base64.StdEncoding.DecodeString(s)
	case Hex:
		return hex.DecodeString(s)
	case Uint64BE:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("uint64-be format: %w", err)
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, n)
		return buf, nil
	default:
		return nil, fmt.Errorf("unknown format %q (want text, base64, hex, or uint64-be)", f)
	}
}
