// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"bytes"
	"math"
	"strconv"
)

// JSONOptions controls JSON output.
type JSONOptions struct {
	// CRLF uses "\r\n" line endings, as the Windows build of xcom2json does.
	// Otherwise "\n" is used.
	CRLF bool
	// ExactFloats writes floats with enough digits to reproduce the exact
	// value. By default floats are written with 6 significant digits, like
	// xcom2json, which can lose precision in the last digit.
	ExactFloats bool
}

// MarshalXComJSON converts the save to the JSON format produced by xcom2json.
// With default options the output is identical to xcom2json's (except for
// line endings; see JSONOptions.CRLF).
func (s *Save) MarshalXComJSON(opts JSONOptions) []byte {
	w := &jsonWriter{skipIndent: true, opts: opts, nl: "\n"}
	if opts.CRLF {
		w.nl = "\r\n"
	}
	w.out.Grow(1 << 20)
	w.save(s)
	return w.out.Bytes()
}

// jsonWriter reproduces the formatting state machine of xcom2json.
type jsonWriter struct {
	out         bytes.Buffer
	indentLevel int
	skipIndent  bool
	needsComma  bool
	opts        JSONOptions
	nl          string
}

func (w *jsonWriter) indent() {
	if w.needsComma {
		w.out.WriteString(", ")
	}
	if !w.skipIndent {
		w.out.WriteString(w.nl)
		for i := 0; i < w.indentLevel; i++ {
			w.out.WriteString("  ")
		}
	}
}

func (w *jsonWriter) begin(open string, omitNewline bool) {
	w.indent()
	w.out.WriteString(open)
	w.indentLevel++
	w.needsComma = false
	w.skipIndent = omitNewline
}

func (w *jsonWriter) end(close string) {
	w.indentLevel--
	if w.needsComma {
		w.out.WriteByte(' ')
	}
	w.needsComma = false
	w.indent()
	w.out.WriteString(close)
	w.needsComma = true
	w.skipIndent = false
}

func (w *jsonWriter) beginObject(omitNewline bool) { w.begin("{ ", omitNewline) }
func (w *jsonWriter) endObject()                   { w.end("}") }
func (w *jsonWriter) beginArray(omitNewline bool)  { w.begin("[ ", omitNewline) }
func (w *jsonWriter) endArray()                    { w.end("]") }

func (w *jsonWriter) endItem(omitNewline bool) {
	w.skipIndent = omitNewline
	w.needsComma = true
}

func (w *jsonWriter) key(name string) {
	w.indent()
	w.out.WriteByte('"')
	w.out.WriteString(name)
	w.out.WriteString(`": `)
	w.skipIndent = true
	w.needsComma = false
}

func (w *jsonWriter) int(name string, v int32, omitNewline bool) {
	w.key(name)
	w.out.WriteString(strconv.Itoa(int(v)))
	w.endItem(omitNewline)
}

func (w *jsonWriter) rawInt(v int32, omitNewline bool) {
	w.indent()
	w.out.WriteString(strconv.Itoa(int(v)))
	w.endItem(omitNewline)
}

func (w *jsonWriter) formatFloat(f float32) string {
	switch {
	case math.IsNaN(float64(f)):
		return "nan"
	case math.IsInf(float64(f), 1):
		return "inf"
	case math.IsInf(float64(f), -1):
		return "-inf"
	}
	if w.opts.ExactFloats {
		return strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	// C++ iostream default: like printf("%g"), 6 significant digits.
	return strconv.FormatFloat(float64(f), 'g', 6, 64)
}

func (w *jsonWriter) float(name string, v float32, omitNewline bool) {
	w.key(name)
	if v == 0 {
		v = 0 // turn -0 into 0, as xcom2json's "val + 0.0f" does
	}
	w.out.WriteString(w.formatFloat(v))
	w.endItem(omitNewline)
}

func (w *jsonWriter) rawFloat(v float32, omitNewline bool) {
	w.indent()
	w.out.WriteString(w.formatFloat(v))
	w.endItem(omitNewline)
}

func (w *jsonWriter) string(name, v string, omitNewline bool) {
	w.key(name)
	w.quoted(v)
	w.endItem(omitNewline)
}

func (w *jsonWriter) rawString(v string, omitNewline bool) {
	w.indent()
	w.quoted(v)
	w.endItem(omitNewline)
}

const hexDigits = "0123456789abcdef"

func (w *jsonWriter) quoted(s string) {
	w.out.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			w.out.WriteString(`\"`)
		case c == '\\':
			w.out.WriteString(`\\`)
		case c == '\n':
			w.out.WriteString(`\n`)
		case c == '\r':
			w.out.WriteString(`\r`)
		case c == '\t':
			w.out.WriteString(`\t`)
		case c > 0 && c < ' ':
			w.out.WriteString(`\u00`)
			w.out.WriteByte(hexDigits[c>>4])
			w.out.WriteByte(hexDigits[c&15])
		default:
			w.out.WriteByte(c)
		}
	}
	w.out.WriteByte('"')
}

func (w *jsonWriter) bool(name string, v bool, omitNewline bool) {
	w.key(name)
	w.out.WriteString(strconv.FormatBool(v))
	w.endItem(omitNewline)
}

func (w *jsonWriter) xstring(s String) {
	w.beginObject(true)
	w.string("str", s.Str, true)
	w.bool("is_wide", s.IsWide, true)
	w.endObject()
}

func (w *jsonWriter) unicodeString(name string, s String) {
	w.key(name)
	w.xstring(s)
}

func hexString(b []byte) string {
	out := make([]byte, 2*len(b))
	for i, c := range b {
		out[2*i] = hexDigits[c>>4]
		out[2*i+1] = hexDigits[c&15]
	}
	return string(out)
}

func (w *jsonWriter) common(p Property, omitNewline bool) {
	w.string("name", p.PropertyName(), omitNewline)
	w.string("kind", p.Kind(), omitNewline)
}

func (w *jsonWriter) property(p Property) {
	switch p := p.(type) {
	case *IntProperty:
		w.beginObject(true)
		w.common(p, true)
		w.int("value", p.Value, true)
		w.endObject()
	case *FloatProperty:
		w.beginObject(true)
		w.common(p, true)
		w.float("value", p.Value, true)
		w.endObject()
	case *BoolProperty:
		w.beginObject(true)
		w.common(p, true)
		w.bool("value", p.Value, true)
		w.endObject()
	case *StrProperty:
		w.beginObject(true)
		w.common(p, true)
		w.unicodeString("value", p.Value)
		w.endObject()
	case *NameProperty:
		w.beginObject(true)
		w.common(p, true)
		w.string("string", p.Value, true)
		w.int("number", p.Number, true)
		w.endObject()
	case *ObjectProperty:
		w.beginObject(true)
		w.common(p, true)
		w.int("actor", p.Actor, true)
		w.endObject()
	case *ByteProperty:
		w.beginObject(false)
		w.common(p, false)
		w.string("type", p.Type, false)
		w.string("value", p.Value, false)
		w.int("number", p.Number, false)
		w.endObject()
	case *StructProperty:
		w.beginObject(false)
		w.common(p, false)
		w.string("struct_name", p.StructName, false)
		if len(p.NativeData) > 0 {
			w.string("native_data", hexString(p.NativeData), false)
			w.key("properties")
			w.beginArray(true)
			w.endArray()
		} else {
			w.string("native_data", "", false)
			w.key("properties")
			w.beginArray(false)
			for _, c := range p.Properties {
				w.property(c)
			}
			w.endArray()
		}
		w.endObject()
	case *ArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.int("data_length", p.DataLength, false)
		w.int("array_bound", p.ArrayBound, false)
		data := ""
		if p.ArrayBound > 0 {
			data = hexString(p.Data)
		}
		w.string("data", data, false)
		w.endObject()
	case *ObjectArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.key("actors")
		w.beginArray(true)
		for _, a := range p.Actors {
			w.rawInt(a, true)
		}
		w.endArray()
		w.endObject()
	case *NumberArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.key("elements")
		w.beginArray(true)
		for _, e := range p.Elements {
			w.rawInt(e, true)
		}
		w.endArray()
		w.endObject()
	case *StringArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.key("strings")
		w.beginArray(false)
		for _, s := range p.Strings {
			w.xstring(s)
		}
		w.endArray()
		w.endObject()
	case *EnumArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.key("enum_values")
		w.beginArray(false)
		for _, e := range p.Values {
			w.beginObject(true)
			w.string("value", e.Value, true)
			w.int("number", e.Number, true)
			w.endObject()
		}
		w.endArray()
		w.endObject()
	case *StructArrayProperty:
		w.beginObject(false)
		w.common(p, false)
		w.key("structs")
		w.beginArray(false)
		for _, props := range p.Structs {
			w.beginArray(false)
			for _, c := range props {
				w.property(c)
			}
			w.endArray()
		}
		w.endArray()
		w.endObject()
	case *StaticArrayProperty:
		w.staticArray(p)
	}
}

func (w *jsonWriter) staticArray(p *StaticArrayProperty) {
	w.beginObject(false)
	w.common(p, false)
	allInts, allNarrowStrings := len(p.Properties) > 0, len(p.Properties) > 0
	for _, e := range p.Properties {
		if _, ok := e.(*IntProperty); !ok {
			allInts = false
		}
		if s, ok := e.(*StrProperty); !ok || s.Value.IsWide {
			allNarrowStrings = false
		}
	}
	switch {
	case allInts:
		w.key("int_values")
		w.beginArray(true)
		for _, e := range p.Properties {
			w.rawInt(e.(*IntProperty).Value, true)
		}
		w.endArray()
	case allNarrowStrings:
		w.key("string_values")
		w.beginArray(true)
		for _, e := range p.Properties {
			w.rawString(e.(*StrProperty).Value.Str, true)
		}
		w.endArray()
	default:
		w.key("properties")
		w.beginArray(false)
		for _, e := range p.Properties {
			w.property(e)
		}
		w.endArray()
	}
	w.endObject()
}

func (w *jsonWriter) checkpoint(c *Checkpoint) {
	w.beginObject(false)
	w.string("name", c.Name, false)
	w.string("instance_name", c.InstanceName, false)
	w.string("class_name", c.ClassName, false)
	w.key("vector")
	w.beginArray(true)
	for _, f := range c.Vector {
		w.rawFloat(f, true)
	}
	w.endArray()
	w.key("rotator")
	w.beginArray(true)
	for _, r := range c.Rotator {
		w.rawInt(r, true)
	}
	w.endArray()
	w.key("properties")
	w.beginArray(false)
	for _, p := range c.Properties {
		w.property(p)
	}
	w.endArray()
	w.int("template_index", c.TemplateIndex, false)
	w.int("pad_size", int32(c.PadSize), false)
	w.endObject()
}

func (w *jsonWriter) chunk(c *CheckpointChunk) {
	w.beginObject(false)
	w.int("unknown_int1", c.UnknownInt1, false)
	w.string("game_type", c.GameType, false)
	w.key("checkpoint_table")
	w.beginArray(false)
	for i := range c.Checkpoints {
		w.checkpoint(&c.Checkpoints[i])
	}
	w.endArray()
	w.int("unknown_int2", c.UnknownInt2, false)
	w.string("class_name", c.ClassName, false)
	w.key("actor_table")
	w.beginArray(false)
	for _, a := range c.Actors {
		w.rawString(a, false)
	}
	w.endArray()
	w.int("unknown_int3", c.UnknownInt3, false)
	w.string("display_name", c.DisplayName, false)
	w.string("map_name", c.MapName, false)
	w.int("unknown_int4", c.UnknownInt4, false)
	w.endObject()
}

func (w *jsonWriter) save(s *Save) {
	h := &s.Header
	w.beginObject(false)
	w.key("header")
	w.beginObject(false)
	w.int("version", int32(h.Version), false)
	w.int("uncompressed_size", h.UncompressedSize, false)
	w.int("game_number", h.GameNumber, false)
	w.int("save_number", h.SaveNumber, false)
	w.unicodeString("save_description", h.SaveDescription)
	w.unicodeString("time", h.Time)
	w.string("map_command", h.MapCommand, false)
	w.bool("tactical_save", h.TacticalSave, false)
	w.bool("ironman", h.Ironman, false)
	w.bool("autosave", h.Autosave, false)
	w.string("dlc", h.DLC, false)
	w.string("language", h.Language, false)
	if h.Version == EnemyWithinAndroid {
		w.int("profile_number", h.ProfileNumber, false)
		w.unicodeString("profile_date", h.ProfileDate)
	}
	w.endObject()

	w.key("actor_table")
	w.beginArray(false)
	for _, a := range s.Actors {
		w.rawString(a, false)
	}
	w.endArray()

	w.key("checkpoints")
	w.beginArray(false)
	for i := range s.Checkpoints {
		w.chunk(&s.Checkpoints[i])
		w.endItem(false)
	}
	w.endArray()
	w.endObject()
}
