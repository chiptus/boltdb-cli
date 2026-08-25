package valuefmt_test

import (
	"testing"

	"github.com/chiptus/boltdb-cli/internal/valuefmt"
)

func TestParseKnownFormats(t *testing.T) {
	cases := map[string]valuefmt.Format{
		"":          valuefmt.Text,
		"text":      valuefmt.Text,
		"base64":    valuefmt.Base64,
		"hex":       valuefmt.Hex,
		"uint64-be": valuefmt.Uint64BE,
	}
	for in, want := range cases {
		got, err := valuefmt.Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseUnknownFormat(t *testing.T) {
	if _, err := valuefmt.Parse("nope"); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []struct {
		format valuefmt.Format
		raw    []byte
	}{
		{valuefmt.Text, []byte("hello")},
		{valuefmt.Base64, []byte{0xff, 0x00, 0xfe}},
		{valuefmt.Hex, []byte{0xde, 0xad, 0xbe, 0xef}},
		{valuefmt.Uint64BE, []byte{0, 0, 0, 0, 0, 0, 0, 42}},
	}
	for _, c := range cases {
		s, err := valuefmt.Encode(c.raw, c.format)
		if err != nil {
			t.Fatalf("Encode(%v, %q): %v", c.raw, c.format, err)
		}
		got, err := valuefmt.Decode(s, c.format)
		if err != nil {
			t.Fatalf("Decode(%q, %q): %v", s, c.format, err)
		}
		if string(got) != string(c.raw) {
			t.Fatalf("round trip mismatch: got %v, want %v", got, c.raw)
		}
	}
}

func TestEncodeUint64BEWrongLength(t *testing.T) {
	if _, err := valuefmt.Encode([]byte{1, 2, 3}, valuefmt.Uint64BE); err == nil {
		t.Fatal("expected error for non-8-byte uint64-be value")
	}
}
