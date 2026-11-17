// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
)

const exampleSave = "testdata/save62"
const exampleJSON = "testdata/x-eu-save62.json"

func readExample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(exampleSave)
	if err != nil {
		t.Skip("example save not present")
	}
	return b
}

func TestBinaryRoundTrip(t *testing.T) {
	orig := readExample(t)
	s, err := Read(orig)
	if err != nil {
		t.Fatal(err)
	}
	if s.Header.Version != EnemyUnknown || len(s.Checkpoints) != 2 {
		t.Fatalf("unexpected save: version %v, %d chunks", s.Header.Version, len(s.Checkpoints))
	}
	rawOrig, err := Decompress(orig)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.UncompressedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, rawOrig) {
		i := 0
		for i < len(raw) && i < len(rawOrig) && raw[i] == rawOrig[i] {
			i++
		}
		t.Fatalf("uncompressed body differs at offset 0x%x (len %d vs %d)", i, len(raw), len(rawOrig))
	}
	out, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, orig) {
		i := 0
		for i < len(out) && i < len(orig) && out[i] == orig[i] {
			i++
		}
		t.Fatalf("written save differs from original at offset 0x%x (len %d vs %d)", i, len(out), len(orig))
	}
}

func TestJSONMatchesXcom2json(t *testing.T) {
	s, err := Read(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(exampleJSON)
	if err != nil {
		t.Skip("example JSON not present")
	}
	// The reference is xcom2json.exe's output, stored with LF line endings
	// like every file in the repository.
	got := s.MarshalXComJSON(JSONOptions{})
	if !bytes.Equal(got, want) {
		i := 0
		for i < len(got) && i < len(want) && got[i] == want[i] {
			i++
		}
		lo := max(0, i-80)
		t.Fatalf("JSON differs at byte %d (len %d vs %d)\n got: %q\nwant: %q", i, len(got), len(want),
			got[lo:min(len(got), i+80)], want[lo:min(len(want), i+80)])
	}
}

// TestJSONToBinary is the json2xcom direction: the example JSON converts back
// to a save identical to the original.
func TestJSONToBinary(t *testing.T) {
	orig := readExample(t)
	s, err := ReadJSONFile(exampleJSON)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, orig) {
		t.Fatal("save built from JSON differs from original")
	}
	// And JSON -> model -> JSON is stable.
	js, _ := os.ReadFile(exampleJSON)
	if !bytes.Equal(s.MarshalXComJSON(JSONOptions{}), js) {
		t.Fatal("JSON round trip is not stable")
	}
}

func TestEditSoldier(t *testing.T) {
	s, err := Read(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	var soldier *Checkpoint
	for i := range s.Checkpoints[0].Checkpoints {
		if c := &s.Checkpoints[0].Checkpoints[i]; c.InstanceName == "XGStrategySoldier_0" {
			soldier = c
		}
	}
	ks := Find(soldier.Properties, "m_kSoldier").(*StructProperty)
	Find(ks.Properties, "strFirstName").(*StrProperty).Value = String{Str: "Zoë Ångström"}
	Find(ks.Properties, "strNickName").(*StrProperty).Value = String{Str: "日本", IsWide: true}
	Find(ks.Properties, "iXP").(*IntProperty).Value = 123456

	b, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s2.MarshalXComJSON(JSONOptions{}), s.MarshalXComJSON(JSONOptions{})) {
		t.Fatal("edited save did not round trip")
	}

	// Non-Latin-1 text in a narrow string is an error, not silent corruption.
	Find(ks.Properties, "strLastName").(*StrProperty).Value = String{Str: "日本"}
	if _, err := s.Bytes(); err == nil {
		t.Fatal("expected error for non-Latin-1 narrow string")
	}
}

func TestCorruptSave(t *testing.T) {
	orig := readExample(t)
	bad := append([]byte(nil), orig...)
	bad[5000] ^= 1
	if _, err := Read(bad); err == nil {
		t.Fatal("expected CRC error")
	} else if _, ok := err.(*CRCError); !ok {
		t.Fatalf("expected *CRCError, got %T: %v", err, err)
	}
	for _, n := range []int{0, 3, 100, 1024, 1030, len(orig) - 1} {
		if _, err := Read(orig[:n]); err == nil {
			t.Fatalf("truncated to %d: expected error", n)
		}
	}
}

func TestExactFloats(t *testing.T) {
	s, err := Read(readExample(t))
	if err != nil {
		t.Fatal(err)
	}
	js := s.MarshalXComJSON(JSONOptions{ExactFloats: true})
	s2, err := UnmarshalXComJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.UncompressedBytes()
	b, _ := s2.UncompressedBytes()
	if !bytes.Equal(a, b) {
		t.Fatal("ExactFloats JSON did not round trip exactly")
	}
}

// TestCorruptBodyNoPanic flips random bytes in the decompressed body and
// checks parsing fails cleanly (or succeeds) without panicking.
func TestCorruptBodyNoPanic(t *testing.T) {
	orig := readExample(t)
	hdr, err := ReadHeader(orig)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Decompress(orig)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 300; i++ {
		bad := append([]byte(nil), raw...)
		for j := 0; j < 1+r.Intn(4); j++ {
			bad[r.Intn(len(bad))] = byte(r.Intn(256))
		}
		func() {
			var err error
			defer recoverFormatError(&err)
			parseBody(hdr, bad)
		}()
	}
}
