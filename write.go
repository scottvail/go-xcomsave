// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/scottvail/go-xcomsave/lzo"
)

// WriteFile writes the save in binary form.
func (s *Save) WriteFile(path string) error {
	b, err := s.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Bytes serializes the save in binary (compressed) form.
func (s *Save) Bytes() ([]byte, error) {
	raw, err := s.UncompressedBytes()
	if err != nil {
		return nil, err
	}
	out := make([]byte, compressedDataStart, compressedDataStart+len(raw)/2)
	for start := 0; start < len(raw); start += chunkSize {
		chunk := raw[start:min(start+chunkSize, len(raw))]
		c, err := compressBlock(s.Header.Version, chunk)
		if err != nil {
			return nil, err
		}
		for _, v := range []uint32{upkMagic, chunkSize, uint32(len(c)), uint32(len(chunk)), uint32(len(c)), uint32(len(chunk))} {
			out = le.AppendUint32(out, v)
		}
		out = append(out, c...)
	}
	if err := writeHeader(out, &s.Header); err != nil {
		return nil, err
	}
	return out, nil
}

// UncompressedBytes serializes the save body (actor table and checkpoint
// chunks) without the header or compression.
func (s *Save) UncompressedBytes() ([]byte, error) {
	if !s.Header.Version.Supported() {
		return nil, &UnsupportedVersionError{s.Header.Version}
	}
	w := &writer{b: make([]byte, 0, 1<<20)}
	v := s.Header.Version
	if err := writeActorTable(w, s.Actors, v); err != nil {
		return nil, err
	}
	for i := range s.Checkpoints {
		if err := writeCheckpointChunk(w, &s.Checkpoints[i], v); err != nil {
			return nil, err
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.b, nil
}

func compressBlock(v Version, chunk []byte) ([]byte, error) {
	if v != EnemyWithinAndroid {
		return lzo.Compress(chunk), nil
	}
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	zw.Write(chunk)
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeHeader writes the header into the first 1024 bytes of out, which
// must already contain the compressed data from offset 1024.
func writeHeader(out []byte, h *Header) error {
	w := &writer{b: make([]byte, 0, compressedDataStart)}
	w.int32(int32(h.Version))
	w.int32(0) // uncompressed size: always written as 0
	w.int32(h.GameNumber)
	w.int32(h.SaveNumber)
	w.unicodeString(h.SaveDescription)
	w.unicodeString(h.Time)
	w.string(h.MapCommand)
	w.bool32(h.TacticalSave)
	w.bool32(h.Ironman)
	w.bool32(h.Autosave)
	w.string(h.DLC)
	w.string(h.Language)
	w.int32(int32(crc32b(out[compressedDataStart:])))
	if h.Version == EnemyWithinAndroid {
		w.raw(make([]byte, 12))
		w.int32(h.ProfileNumber)
		w.unicodeString(h.ProfileDate)
	}
	if w.err != nil {
		return w.err
	}
	if len(w.b) > 1016 {
		return fmt.Errorf("header too large (%d bytes)", len(w.b))
	}
	copy(out, w.b)
	if h.Version != EnemyWithinAndroid {
		// The header CRC covers the header plus the 4 bytes following it.
		hdrLen := len(w.b) + 4
		le.PutUint32(out[1016:], uint32(hdrLen))
		le.PutUint32(out[1020:], crc32b(out[:hdrLen]))
	}
	return nil
}

// splitActorName splits "pkg.Class_N" (or "Class_N" for Enemy Unknown) into
// its parts; the stored instance number is N+1.
func splitActorName(name string, v Version) (pkg, class string, instance int32, err error) {
	us := strings.LastIndexByte(name, '_')
	if us < 0 {
		return "", "", 0, fmt.Errorf("malformed actor name %q: no instance number", name)
	}
	n, err := strconv.Atoi(name[us+1:])
	if err != nil {
		return "", "", 0, fmt.Errorf("malformed actor name %q: %v", name, err)
	}
	class = name[:us]
	if v != EnemyUnknown {
		dot := strings.IndexByte(name, '.')
		if dot < 0 || dot > us {
			return "", "", 0, fmt.Errorf("malformed actor name %q: expected package.Class_N", name)
		}
		pkg, class = name[:dot], name[dot+1:us]
	}
	return pkg, class, int32(n + 1), nil
}

func writeActorTable(w *writer, actors []string, v Version) error {
	if v == EnemyUnknown {
		w.int32(int32(len(actors)))
	} else {
		w.int32(int32(len(actors) * 2))
	}
	for _, a := range actors {
		pkg, class, inst, err := splitActorName(a, v)
		if err != nil {
			return err
		}
		w.string(class)
		w.int32(inst)
		if v != EnemyUnknown {
			w.string(pkg)
			w.int32(0)
		}
	}
	return nil
}

func writeCheckpointChunk(w *writer, c *CheckpointChunk, v Version) error {
	w.int32(c.UnknownInt1)
	w.string(c.GameType)
	w.string("None")
	w.int32(c.UnknownInt2)
	w.int32(int32(len(c.Checkpoints)))
	for i := range c.Checkpoints {
		if err := writeCheckpoint(w, &c.Checkpoints[i], v); err != nil {
			return err
		}
	}
	w.int32(int32(len(c.NameTable)))
	for _, e := range c.NameTable {
		w.string(e.Name)
		w.raw(make([]byte, 8))
		w.int32(int32(len(e.Data)))
		w.raw(e.Data)
	}
	w.string(c.ClassName)
	if err := writeActorTable(w, c.Actors, v); err != nil {
		return err
	}
	w.int32(c.UnknownInt3)
	w.int32(int32(len(c.ActorTemplates)))
	for _, t := range c.ActorTemplates {
		w.string(t.ActorClassPath)
		w.raw(t.LoadParams[:])
		w.string(t.ArchetypePath)
	}
	w.string(c.DisplayName)
	w.string(c.MapName)
	w.int32(c.UnknownInt4)
	return nil
}

func writeCheckpoint(w *writer, c *Checkpoint, v Version) error {
	w.string(c.Name)
	w.string(c.InstanceName)
	for _, f := range c.Vector {
		w.float32(f)
	}
	for _, i := range c.Rotator {
		w.int32(i)
	}
	w.string(c.ClassName)
	lenPos := len(w.b)
	w.int32(0) // patched below
	start := len(w.b)
	if err := writeProperties(w, c.Properties, v); err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	w.raw(make([]byte, c.PadSize))
	le.PutUint32(w.b[lenPos:], uint32(len(w.b)-start))
	w.int32(c.TemplateIndex)
	return nil
}

// writeProperties writes a property list terminated by "None".
func writeProperties(w *writer, props []Property, v Version) error {
	for _, p := range props {
		if err := writeProperty(w, p, 0, v); err != nil {
			return err
		}
	}
	w.string("None")
	w.int32(0)
	return nil
}

func writeProperty(w *writer, p Property, arrayIndex int32, v Version) error {
	if sa, ok := p.(*StaticArrayProperty); ok {
		// Static arrays are written as their elements, with increasing
		// array indexes.
		for i, e := range sa.Properties {
			if _, nested := e.(*StaticArrayProperty); nested {
				return fmt.Errorf("%s: nested static array", sa.Name)
			}
			if err := writeProperty(w, e, int32(i), v); err != nil {
				return err
			}
		}
		return nil
	}

	w.string(p.PropertyName())
	w.int32(0)
	w.string(p.Kind())
	w.int32(0)
	sizePos := len(w.b)
	w.int32(0) // size, patched below
	w.int32(arrayIndex)
	// The size covers the value only; for structs and enums, the type name
	// that precedes the value is not counted, and bools always have size 0.
	valueStart := len(w.b)

	switch p := p.(type) {
	case *IntProperty:
		w.int32(p.Value)
	case *FloatProperty:
		w.float32(p.Value)
	case *BoolProperty:
		if p.Value {
			w.byte(1)
		} else {
			w.byte(0)
		}
		valueStart = len(w.b)
	case *StrProperty:
		w.unicodeString(p.Value)
	case *NameProperty:
		w.string(p.Value)
		w.int32(p.Number)
	case *ObjectProperty:
		writeActorRef(w, p.Actor, v == EnemyUnknown)
	case *ByteProperty:
		w.string(p.Type)
		w.int32(0)
		valueStart = len(w.b)
		if p.Type == "None" {
			w.byte(byte(p.Number))
		} else {
			w.string(p.Value)
			w.int32(p.Number)
		}
	case *StructProperty:
		w.string(p.StructName)
		w.int32(0)
		valueStart = len(w.b)
		if len(p.NativeData) > 0 {
			w.raw(p.NativeData)
		} else if err := writeProperties(w, p.Properties, v); err != nil {
			return err
		}
	case *ArrayProperty:
		w.int32(p.ArrayBound)
		switch {
		case int(p.DataLength) == len(p.Data):
			w.raw(p.Data)
		case len(p.Data) == 0 && p.DataLength > 0:
			w.raw(make([]byte, p.DataLength))
		case p.DataLength < 0:
			// Degenerate size < 4 from the original save; reproduce it.
			le.PutUint32(w.b[sizePos:], uint32(4+p.DataLength))
			return nil
		default:
			return fmt.Errorf("%s: data_length %d does not match %d bytes of data", p.Name, p.DataLength, len(p.Data))
		}
	case *ObjectArrayProperty:
		w.int32(int32(len(p.Actors)))
		for _, a := range p.Actors {
			writeActorRef(w, a, false)
		}
	case *NumberArrayProperty:
		w.int32(int32(len(p.Elements)))
		for _, e := range p.Elements {
			w.int32(e)
		}
	case *StringArrayProperty:
		w.int32(int32(len(p.Strings)))
		for _, s := range p.Strings {
			w.unicodeString(s)
		}
	case *EnumArrayProperty:
		w.int32(int32(len(p.Values)))
		for _, e := range p.Values {
			w.string(e.Value)
			w.int32(e.Number)
		}
	case *StructArrayProperty:
		w.int32(int32(len(p.Structs)))
		for _, props := range p.Structs {
			if err := writeProperties(w, props, v); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported property type %T", p)
	}
	le.PutUint32(w.b[sizePos:], uint32(len(w.b)-valueStart))
	return nil
}

// writeActorRef writes an actor reference. Enemy Within stores each
// reference as two ints (2n+1, 2n); Enemy Unknown object properties store a
// single int.
func writeActorRef(w *writer, actor int32, single bool) {
	switch {
	case single:
		w.int32(actor)
	case actor == -1:
		w.int32(-1)
		w.int32(-1)
	default:
		w.int32(actor*2 + 1)
		w.int32(actor * 2)
	}
}
