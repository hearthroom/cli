// Package importer converts cards from other tools (SillyTavern PNG / JSON /
// CHARX, the MMD three-file set) into card folders. It never talks to the
// network; everything it could not place is reported, not dropped.
//
// The parsers are ports of the community site's importers so a card imports
// the same way in the browser and in the CLI.
package importer

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
)

var pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// Errors shared by the parsers. They are stable codes so callers can map them.
var (
	ErrNotPNG       = errors.New("not_png")
	ErrPNGTruncated = errors.New("png_truncated")
	ErrInvalid      = errors.New("tavern_invalid")
	ErrNoMetadata   = errors.New("tavern_no_metadata")
)

// IsPNG reports whether bytes start with the PNG signature.
func IsPNG(b []byte) bool {
	return len(b) >= 8 && bytes.Equal(b[:8], pngSignature)
}

type pngChunk struct {
	typ  string
	data []byte
}

// readChunks splits a PNG into chunks. Length fields are untrusted input and
// are checked against the real buffer before any slice is taken.
func readChunks(b []byte) ([]pngChunk, error) {
	if !IsPNG(b) {
		return nil, ErrNotPNG
	}
	var chunks []pngChunk
	at := 8
	for at+8 <= len(b) {
		length := int(binary.BigEndian.Uint32(b[at : at+4]))
		end := at + 12 + length
		if length < 0 || length > len(b) || end > len(b) {
			return nil, ErrPNGTruncated
		}
		typ := string(b[at+4 : at+8])
		chunks = append(chunks, pngChunk{typ: typ, data: b[at+8 : at+8+length]})
		at = end
		if typ == "IEND" {
			break
		}
	}
	return chunks, nil
}

// readTextChunk returns the Latin-1 text of the first tEXt chunk whose
// keyword matches (case-insensitively), or "" and false.
func readTextChunk(b []byte, keyword string) (string, bool, error) {
	chunks, err := readChunks(b)
	if err != nil {
		return "", false, err
	}
	want := strings.ToLower(keyword)
	for _, c := range chunks {
		if c.typ != "tEXt" {
			continue
		}
		nul := bytes.IndexByte(c.data, 0)
		if nul < 0 {
			continue
		}
		if strings.ToLower(latin1(c.data[:nul])) == want {
			return latin1(c.data[nul+1:]), true, nil
		}
	}
	return "", false, nil
}

// latin1 maps each byte to the rune with the same value.
func latin1(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// decodeBase64UTF8 decodes the base64 text of a tEXt chunk into UTF-8 bytes.
func decodeBase64UTF8(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}

// pngCardJSON extracts the card JSON from a PNG (ccv3 first, then chara).
func pngCardJSON(b []byte) ([]byte, error) {
	for _, kw := range []string{"ccv3", "chara"} {
		text, ok, err := readTextChunk(b, kw)
		if err != nil {
			return nil, err
		}
		if ok && strings.TrimSpace(text) != "" {
			raw, err := decodeBase64UTF8(text)
			if err != nil {
				return nil, ErrInvalid
			}
			return raw, nil
		}
	}
	return nil, ErrNoMetadata
}
