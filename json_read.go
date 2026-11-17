// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
)

// ReadJSONFile reads a save from a file in xcom2json's JSON format.
func ReadJSONFile(path string) (*Save, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return UnmarshalXComJSON(b)
}

// JSONError reports JSON that does not have the expected shape.
type JSONError struct {
	Node string // what was being decoded, e.g. "checkpoint"
	Msg  string
}

func (e *JSONError) Error() string {
	return fmt.Sprintf("invalid json format in %s: %s", e.Node, e.Msg)
}

// UnmarshalXComJSON parses a save in xcom2json's JSON format.
func UnmarshalXComJSON(data []byte) (save *Save, err error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var root any
	if err := d.Decode(&root); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *JSONError:
				save, err = nil, e
			case *UnsupportedVersionError:
				save, err = nil, e
			default:
				panic(r)
			}
		}
	}()
	return buildSave(obj(root, "root")), nil
}

// jobj is a JSON object being decoded; node names the object in errors.
type jobj struct {
	m    map[string]any
	node string
}

func obj(v any, node string) jobj {
	m, ok := v.(map[string]any)
	if !ok {
		panic(&JSONError{node, "expected an object"})
	}
	return jobj{m, node}
}

func (o jobj) fail(format string, a ...any) {
	panic(&JSONError{o.node, fmt.Sprintf(format, a...)})
}

func (o jobj) has(key string) bool {
	v, ok := o.m[key]
	return ok && v != nil
}

func (o jobj) get(key string) any {
	v, ok := o.m[key]
	if !ok || v == nil {
		o.fail("missing %q", key)
	}
	return v
}

func (o jobj) str(key string) string {
	s, ok := o.get(key).(string)
	if !ok {
		o.fail("%q must be a string", key)
	}
	return s
}

func (o jobj) boolean(key string) bool {
	b, ok := o.get(key).(bool)
	if !ok {
		o.fail("%q must be a boolean", key)
	}
	return b
}

func (o jobj) int(key string) int32 {
	n, ok := toInt(o.get(key))
	if !ok {
		o.fail("%q must be a number", key)
	}
	return n
}

func (o jobj) float(key string) float32 {
	f, ok := toFloat(o.get(key))
	if !ok {
		o.fail("%q must be a number", key)
	}
	return f
}

func (o jobj) arr(key string) []any {
	a, ok := o.get(key).([]any)
	if !ok {
		o.fail("%q must be an array", key)
	}
	return a
}

func (o jobj) obj(key string) jobj { return obj(o.get(key), o.node+"."+key) }

func (o jobj) xstring(key string) String { return xstring(o.get(key), o.node+"."+key) }

func xstring(v any, node string) String {
	o := obj(v, node)
	return String{Str: o.str("str"), IsWide: o.boolean("is_wide")}
}

func toFloat(v any) (float32, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil {
		return 0, false
	}
	return float32(f), true
}

func toInt(v any) (int32, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
		if i < math.MinInt32 || i > math.MaxUint32 {
			return 0, false
		}
		return int32(i), true // values up to MaxUint32 wrap, e.g. CRCs
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || f < math.MinInt32 || f > math.MaxInt32 {
		return 0, false
	}
	return int32(f), true
}

func ints(a []any, o jobj, key string) []int32 {
	out := make([]int32, 0, len(a))
	for _, v := range a {
		n, ok := toInt(v)
		if !ok {
			o.fail("%q must contain only numbers", key)
		}
		out = append(out, n)
	}
	return out
}

func buildSave(o jobj) *Save {
	s := &Save{}
	s.Header = buildHeader(o.obj("header"))
	s.Actors = buildActorTable(o, "actor_table")
	for i, c := range o.arr("checkpoints") {
		s.Checkpoints = append(s.Checkpoints, buildChunk(obj(c, fmt.Sprintf("checkpoints[%d]", i))))
	}
	return s
}

func buildHeader(o jobj) Header {
	var h Header
	h.Version = Version(uint32(o.int("version")))
	if !h.Version.Supported() {
		panic(&UnsupportedVersionError{h.Version})
	}
	h.UncompressedSize = o.int("uncompressed_size")
	h.GameNumber = o.int("game_number")
	h.SaveNumber = o.int("save_number")
	h.SaveDescription = o.xstring("save_description")
	h.Time = o.xstring("time")
	h.MapCommand = o.str("map_command")
	h.TacticalSave = o.boolean("tactical_save")
	h.Ironman = o.boolean("ironman")
	h.Autosave = o.boolean("autosave")
	h.DLC = o.str("dlc")
	h.Language = o.str("language")
	if h.Version == EnemyWithinAndroid {
		h.ProfileNumber = o.int("profile_number")
		h.ProfileDate = o.xstring("profile_date")
	}
	return h
}

func buildActorTable(o jobj, key string) []string {
	var out []string
	for _, v := range o.arr(key) {
		s, ok := v.(string)
		if !ok {
			o.fail("%q must contain only strings", key)
		}
		out = append(out, s)
	}
	return out
}

func buildChunk(o jobj) CheckpointChunk {
	var c CheckpointChunk
	c.UnknownInt1 = o.int("unknown_int1")
	c.GameType = o.str("game_type")
	for i, v := range o.arr("checkpoint_table") {
		c.Checkpoints = append(c.Checkpoints, buildCheckpoint(obj(v, fmt.Sprintf("%s.checkpoint_table[%d]", o.node, i))))
	}
	c.UnknownInt2 = o.int("unknown_int2")
	c.ClassName = o.str("class_name")
	c.Actors = buildActorTable(o, "actor_table")
	c.UnknownInt3 = o.int("unknown_int3")
	c.DisplayName = o.str("display_name")
	c.MapName = o.str("map_name")
	c.UnknownInt4 = o.int("unknown_int4")
	return c
}

func buildCheckpoint(o jobj) Checkpoint {
	var c Checkpoint
	c.Name = o.str("name")
	o.node = c.Name
	c.InstanceName = o.str("instance_name")
	vec, rot := o.arr("vector"), o.arr("rotator")
	if len(vec) != 3 || len(rot) != 3 {
		o.fail("vector and rotator must have 3 items")
	}
	for i := range 3 {
		var ok bool
		if c.Vector[i], ok = toFloat(vec[i]); !ok {
			o.fail("vector must contain numbers")
		}
		if c.Rotator[i], ok = toInt(rot[i]); !ok {
			o.fail("rotator must contain numbers")
		}
	}
	c.ClassName = o.str("class_name")
	c.Properties = buildPropertyList(o.arr("properties"), o.node)
	c.TemplateIndex = o.int("template_index")
	pad := o.int("pad_size")
	if pad < 0 {
		o.fail("pad_size must not be negative")
	}
	c.PadSize = uint32(pad)
	return c
}

func buildPropertyList(a []any, node string) []Property {
	props := make([]Property, 0, len(a))
	for i, v := range a {
		props = append(props, buildProperty(obj(v, fmt.Sprintf("%s[%d]", node, i))))
	}
	return props
}

func buildProperty(o jobj) Property {
	name := o.str("name")
	o.node += " (" + name + ")"
	switch kind := o.str("kind"); kind {
	case "IntProperty":
		return &IntProperty{name, o.int("value")}
	case "FloatProperty":
		return &FloatProperty{name, o.float("value")}
	case "BoolProperty":
		return &BoolProperty{name, o.boolean("value")}
	case "StrProperty":
		return &StrProperty{name, o.xstring("value")}
	case "NameProperty":
		return &NameProperty{name, o.str("string"), o.int("number")}
	case "ObjectProperty":
		return &ObjectProperty{name, o.int("actor")}
	case "ByteProperty":
		return &ByteProperty{name, o.str("type"), o.str("value"), o.int("number")}
	case "StructProperty":
		p := &StructProperty{Name: name, StructName: o.str("struct_name")}
		if nd := o.str("native_data"); nd != "" {
			b, err := hex.DecodeString(nd)
			if err != nil {
				o.fail("native_data: %v", err)
			}
			p.NativeData = b
		} else {
			p.Properties = buildPropertyList(o.arr("properties"), o.node)
		}
		return p
	case "ArrayProperty":
		return buildArrayProperty(o, name)
	case "StaticArrayProperty":
		sa := &StaticArrayProperty{Name: name}
		switch {
		case o.has("int_values"):
			for _, n := range ints(o.arr("int_values"), o, "int_values") {
				sa.Properties = append(sa.Properties, &IntProperty{name, n})
			}
		case o.has("string_values"):
			for _, v := range o.arr("string_values") {
				s, ok := v.(string)
				if !ok {
					o.fail("string_values must contain only strings")
				}
				sa.Properties = append(sa.Properties, &StrProperty{name, String{Str: s}})
			}
		default:
			sa.Properties = buildPropertyList(o.arr("properties"), o.node)
		}
		return sa
	default:
		o.fail("unknown property kind %q", kind)
		return nil
	}
}

func buildArrayProperty(o jobj, name string) Property {
	switch {
	case o.has("actors"):
		return &ObjectArrayProperty{name, ints(o.arr("actors"), o, "actors")}
	case o.has("elements"):
		return &NumberArrayProperty{name, ints(o.arr("elements"), o, "elements")}
	case o.has("structs"):
		p := &StructArrayProperty{Name: name}
		for i, v := range o.arr("structs") {
			a, ok := v.([]any)
			if !ok {
				o.fail("structs[%d] must be an array", i)
			}
			p.Structs = append(p.Structs, buildPropertyList(a, fmt.Sprintf("%s.structs[%d]", o.node, i)))
		}
		return p
	case o.has("strings"):
		p := &StringArrayProperty{Name: name}
		for i, v := range o.arr("strings") {
			p.Strings = append(p.Strings, xstring(v, fmt.Sprintf("%s.strings[%d]", o.node, i)))
		}
		return p
	case o.has("enum_values"):
		p := &EnumArrayProperty{Name: name}
		for i, v := range o.arr("enum_values") {
			e := obj(v, fmt.Sprintf("%s.enum_values[%d]", o.node, i))
			p.Values = append(p.Values, EnumValue{e.str("value"), e.int("number")})
		}
		return p
	}
	p := &ArrayProperty{Name: name, DataLength: o.int("data_length"), ArrayBound: o.int("array_bound")}
	if d := o.str("data"); d != "" {
		b, err := hex.DecodeString(d)
		if err != nil {
			o.fail("data: %v", err)
		}
		if len(b) != int(p.DataLength) {
			o.fail("data has %d bytes but data_length is %d", len(b), p.DataLength)
		}
		p.Data = b
	}
	return p
}
