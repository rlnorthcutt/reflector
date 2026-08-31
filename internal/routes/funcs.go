package routes

import (
	"crypto/rand"
	"math/big"
	"strings"
	"text/template"
)

const randStringAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// FuncMap holds the helper functions available to route body templates:
// {{ uuid }}, {{ randInt 1 100 }}, {{ randString 8 }}, {{ repeat "x" 10 }}.
var FuncMap = template.FuncMap{
	"uuid":       newUUID,
	"randInt":    randInt,
	"randString": randString,
	"repeat":     strings.Repeat,
}

// newUUID returns a random RFC 4122 version 4 UUID string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return string(hexDigits(b[0:4])) + "-" +
		string(hexDigits(b[4:6])) + "-" +
		string(hexDigits(b[6:8])) + "-" +
		string(hexDigits(b[8:10])) + "-" +
		string(hexDigits(b[10:16]))
}

const hextable = "0123456789abcdef"

func hexDigits(b []byte) []byte {
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hextable[v>>4]
		out[i*2+1] = hextable[v&0x0f]
	}
	return out
}

// randInt returns a random integer in [min, max], inclusive.
func randInt(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)+1))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

// randString returns a random alphanumeric string of length n.
func randString(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = randStringAlphabet[randInt(0, len(randStringAlphabet)-1)]
	}
	return string(out)
}
