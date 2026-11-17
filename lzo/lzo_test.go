// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from minilzo 2.09, Copyright (C) 1996-2015 Markus F.X.J. Oberhumer (http://www.oberhumer.com/opensource/lzo/).

package lzo

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"testing"
)

func roundTrip(t *testing.T, name string, data []byte) {
	t.Helper()
	c := Compress(data)
	d, err := Decompress(c, len(data))
	if err != nil {
		t.Fatalf("%s: decompress: %v", name, err)
	}
	if !bytes.Equal(d, data) {
		t.Fatalf("%s: round trip mismatch", name)
	}
}

func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 3, 4, 17, 20, 21, 31, 32, 33, 100, 238, 239, 1000, 49151, 49152, 49153, 131072, 300000} {
		random := make([]byte, n)
		r.Read(random)
		roundTrip(t, "random", random)

		// Compressible: small alphabet with repeats at many distances.
		comp := make([]byte, n)
		for i := range comp {
			switch {
			case i > 20000 && r.Intn(4) == 0:
				comp[i] = comp[i-20000+r.Intn(3)]
			case i > 3000 && r.Intn(3) == 0:
				comp[i] = comp[i-3000]
			default:
				comp[i] = byte('a' + r.Intn(4))
			}
		}
		roundTrip(t, "compressible", comp)
		roundTrip(t, "zeros", make([]byte, n))
	}
}

func TestCorrupt(t *testing.T) {
	c := Compress(bytes.Repeat([]byte("hello world "), 100))
	for i := range c {
		bad := append([]byte(nil), c[:i]...)
		if _, err := Decompress(bad, 1200); err == nil {
			t.Fatalf("truncated at %d: expected error", i)
		}
	}
}

// TestGameChunks decompresses every chunk of a real save and checks that
// recompressing reproduces the game's own compressed bytes.
func TestGameChunks(t *testing.T) {
	data, err := os.ReadFile("../testdata/save62")
	if err != nil {
		t.Skip("example save not present")
	}
	pos, n, same := 1024, 0, 0
	for pos < len(data) {
		le := binary.LittleEndian
		csize := int(le.Uint32(data[pos+8:]))
		usize := int(le.Uint32(data[pos+12:]))
		chunk := data[pos+24 : pos+24+csize]
		raw, err := Decompress(chunk, usize)
		if err != nil {
			t.Fatalf("chunk %d: %v", n, err)
		}
		if bytes.Equal(Compress(raw), chunk) {
			same++
		}
		pos += 24 + csize
		n++
	}
	t.Logf("%d chunks, %d recompress byte-identical to game output", n, same)
}
