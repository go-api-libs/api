package union_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/go-api-libs/api/union"
)

func decoder(s string, opts ...jsontext.Options) *jsontext.Decoder {
	return jsontext.NewDecoder(strings.NewReader(s), opts...)
}

func wantErr(t *testing.T, err error, contains string) {
	t.Helper()

	if err == nil {
		t.Fatalf("got no error, want one with %q", contains)
	}

	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("got %v, want one with %q", err, contains)
	}
}

func TestStrict(t *testing.T) {
	if !union.Strict(decoder(`{}`)) {
		t.Error("a decoder told nothing: not strict")
	}

	if union.Strict(decoder(`{}`, json.RejectUnknownMembers(false))) {
		t.Error("a lenient decoder: strict")
	}

	if !union.Strict(decoder(`{}`, json.RejectUnknownMembers(true))) {
		t.Error("a strict decoder: not strict")
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		err      error
		contains string
	}{
		{union.UnknownName("x"), `unknown object member name "x"`},
		{union.Missing("x"), `missing object member name "x"`},
		{union.UnknownValue("type", "y"), `unknown value of "type"`},
	} {
		if _, ok := errors.AsType[*json.SemanticError](tc.err); !ok {
			t.Errorf("%v: not a *json.SemanticError", tc.err)
		}

		wantErr(t, tc.err, tc.contains)
	}

	if !errors.Is(union.UnknownName("x"), json.ErrUnknownName) {
		t.Error("UnknownName is not json.ErrUnknownName")
	}
}

func TestFirstMember(t *testing.T) {
	t.Run("first, the fast way", func(t *testing.T) {
		dec := decoder(`{"type":"a","x":1}`)

		tag, val, rest, err := union.FirstMember(dec, "type")
		if err != nil || tag != "a" || string(val) != `"a"` || rest != dec {
			t.Fatalf("got %q, %s, same decoder %v, %v", tag, val, rest == dec, err)
		}

		if tok, err := rest.ReadToken(); err != nil || tok.String() != "x" {
			t.Fatalf("then got %v, %v, want member x", tok, err)
		}
	})

	t.Run("later, moved to the front", func(t *testing.T) {
		tag, _, rest, err := union.FirstMember(decoder(`{"x":1,"y":[2],"type":"a"}`), "type")
		if err != nil || tag != "a" {
			t.Fatalf("got %q, %v", tag, err)
		}

		var names []string
		for rest.PeekKind() != jsontext.KindEndObject {
			tok, err := rest.ReadToken()
			if err != nil {
				t.Fatal(err)
			}

			names = append(names, tok.String())

			if err := rest.SkipValue(); err != nil {
				t.Fatal(err)
			}
		}

		if !slices.Equal(names, []string{"x", "y"}) {
			t.Errorf("then got %q, want the rest in order", names)
		}
	})

	for _, tc := range []struct {
		name, in, contains string
	}{
		{"not an object", `[1]`, "want an object"},
		{"empty", `{}`, `missing object member name "type"`},
		{"not a string", `{"type":1}`, `member "type" is not a string`},
		{"later, and missing", `{"x":1}`, `missing object member name "type"`},
		{"broken", `{"type"`, "EOF"},
		{"nothing", ``, "EOF"},
		{"broken later", `{"x":1,`, "EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := union.FirstMember(decoder(tc.in), "type")
			wantErr(t, err, tc.contains)
		})
	}
}

// fields decodes the members it is given into a map, and declares only those of declared.
type fields struct {
	declared map[string]bool
	got      map[string]string
	fail     string
}

func (f *fields) decode(dec *jsontext.Decoder, name string) (bool, error) {
	if !f.declared[name] {
		return false, nil
	}

	if name == f.fail {
		return true, errors.New("cannot decode " + name)
	}

	val, err := dec.ReadValue()
	if err != nil {
		return true, err
	}

	if f.got == nil {
		f.got = map[string]string{}
	}

	f.got[name] = string(val)

	return true, nil
}

func TestMembersFrom(t *testing.T) {
	read := func(s string, opts ...jsontext.Options) (*jsontext.Decoder, string, jsontext.Value) {
		dec := decoder(s, opts...)
		_, _ = dec.ReadToken()
		tok, _ := dec.ReadToken()
		name := tok.String()
		val, _ := dec.ReadValue()

		return dec, name, val.Clone()
	}

	t.Run("decodes each member", func(t *testing.T) {
		f := &fields{declared: map[string]bool{"a": true, "b": true}}

		dec, name, val := read(`{"a":1,"b":"x"}`)
		if err := union.MembersFrom(dec, name, val, f.decode); err != nil {
			t.Fatal(err)
		}

		if f.got["a"] != "1" || f.got["b"] != `"x"` {
			t.Errorf("got %v", f.got)
		}
	})

	t.Run("strict, a member not declared", func(t *testing.T) {
		f := &fields{declared: map[string]bool{"a": true}}
		dec, name, val := read(`{"a":1,"c":2}`)
		wantErr(t, union.MembersFrom(dec, name, val, f.decode), `"c"`)

		dec, name, val = read(`{"c":2}`)
		wantErr(t, union.MembersFrom(dec, name, val, f.decode), `"c"`)
	})

	t.Run("leniently, a member not declared is skipped", func(t *testing.T) {
		f := &fields{declared: map[string]bool{"a": true}}

		dec, name, val := read(`{"a":1,"c":{"d":[3]}}`, json.RejectUnknownMembers(false))
		if err := union.MembersFrom(dec, name, val, f.decode); err != nil {
			t.Fatal(err)
		}

		if len(f.got) != 1 {
			t.Errorf("got %v", f.got)
		}
	})

	t.Run("a member that does not decode", func(t *testing.T) {
		f := &fields{declared: map[string]bool{"a": true, "b": true}, fail: "b"}
		dec, name, val := read(`{"a":1,"b":2}`)
		wantErr(t, union.MembersFrom(dec, name, val, f.decode), "cannot decode b")

		f.fail = "a"
		dec, name, val = read(`{"a":1,"b":2}`)
		wantErr(t, union.MembersFrom(dec, name, val, f.decode), "cannot decode a")
	})
}

func TestPartsFrom(t *testing.T) {
	read := func(s string, opts ...jsontext.Options) (*jsontext.Decoder, string, jsontext.Value) {
		dec := decoder(s, opts...)
		_, _ = dec.ReadToken()
		tok, _ := dec.ReadToken()
		name := tok.String()
		val, _ := dec.ReadValue()

		return dec, name, val.Clone()
	}

	parts := func(a, b *fields) []union.Part {
		return []union.Part{{Members: a.declared, Decode: a.decode}, {Members: b.declared, Decode: b.decode}}
	}

	t.Run("each member into the parts that declare it", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"type": true, "id": true}}
		b := &fields{declared: map[string]bool{"type": true, "text": true}}

		dec, name, val := read(`{"type":"p","id":1,"text":"hi"}`)
		if err := union.PartsFrom(dec, name, val, parts(a, b)); err != nil {
			t.Fatal(err)
		}

		if a.got["type"] != `"p"` || b.got["type"] != `"p"` || a.got["id"] != "1" || b.got["text"] != `"hi"` {
			t.Errorf("got %v and %v", a.got, b.got)
		}
	})

	t.Run("a later member several parts declare", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"type": true, "id": true}}
		b := &fields{declared: map[string]bool{"type": true, "id": true}}

		dec, name, val := read(`{"type":"p","id":1}`)
		if err := union.PartsFrom(dec, name, val, parts(a, b)); err != nil {
			t.Fatal(err)
		}

		if a.got["id"] != "1" || b.got["id"] != "1" {
			t.Errorf("got %v and %v", a.got, b.got)
		}
	})

	t.Run("strict, a member no part declares", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"type": true}}
		b := &fields{declared: map[string]bool{}}
		dec, name, val := read(`{"type":"p","new":1}`)
		wantErr(t, union.PartsFrom(dec, name, val, parts(a, b)), `"new"`)
	})

	t.Run("leniently, a member no part declares is skipped", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"id": true}}
		b := &fields{declared: map[string]bool{}}

		dec, name, val := read(`{"type":"p","new":[1],"id":2}`, json.RejectUnknownMembers(false))
		if err := union.PartsFrom(dec, name, val, parts(a, b)); err != nil {
			t.Fatal(err)
		}

		if a.got["id"] != "2" {
			t.Errorf("got %v", a.got)
		}
	})

	t.Run("a member that does not decode", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"type": true, "id": true}, fail: "id"}
		b := &fields{declared: map[string]bool{"type": true, "id": true}}
		dec, name, val := read(`{"type":"p","id":1}`)
		wantErr(t, union.PartsFrom(dec, name, val, parts(a, b)), "cannot decode id")

		a = &fields{declared: map[string]bool{"type": true, "id": true}, fail: "id"}
		b = &fields{declared: map[string]bool{"type": true}}
		dec, name, val = read(`{"type":"p","id":1}`)
		wantErr(t, union.PartsFrom(dec, name, val, parts(a, b)), "cannot decode id")
	})

	t.Run("broken", func(t *testing.T) {
		a := &fields{declared: map[string]bool{"type": true, "id": true}}
		b := &fields{declared: map[string]bool{"id": true}}
		dec, name, val := read(`{"type":"p","id":`)
		wantErr(t, union.PartsFrom(dec, name, val, parts(a, b)), "EOF")

		dec, name, val = read(`{"type":"p",`)
		wantErr(t, union.PartsFrom(dec, name, val, parts(a, b)), "EOF")
	})
}

func TestFirst(t *testing.T) {
	for _, tc := range []struct {
		name, raw, value, want string
	}{
		{"first already", `{"type":"a","x":1}`, "", `{"type":"a","x":1}`},
		{"first with the value", `{"type":"a","x":1}`, "a", `{"type":"a","x":1}`},
		{"later", `{"x":1,"type":"a"}`, "", `{"type":"a","x":1}`},
		{"later with the value", `{"x":1,"type":"a"}`, "a", `{"type":"a","x":1}`},
		{"left empty first", `{"type":"","x":1}`, "a", `{"type":"a","x":1}`},
		{"left empty later", `{"x":1,"type":""}`, "a", `{"type":"a","x":1}`},
		{"left out, the value known", `{"x":1}`, "a", `{"type":"a","x":1}`},
		{"left out of an empty object", `{}`, "a", `{"type":"a"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := union.First(jsontext.Value(tc.raw), "type", tc.value)
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %s, %v, want %s", got, err, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name, raw, value, contains string
	}{
		{"another value first", `{"type":"b"}`, "a", `member "type" is "b", want "a"`},
		{"another value later", `{"x":1,"type":"b"}`, "a", `member "type" is "b", want "a"`},
		{"not a string later", `{"x":1,"type":1}`, "a", `member "type" is not a string`},
		{"left out, the value not known", `{"x":1}`, "", `missing object member name "type"`},
		{"broken", `{"x"`, "", "EOF"},
		{"nothing", ``, "", "EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := union.First(jsontext.Value(tc.raw), "type", tc.value)
			wantErr(t, err, tc.contains)
		})
	}
}

func TestMemberString(t *testing.T) {
	if got, err := union.MemberString(jsontext.Value(`{"x":[1],"type":"a","y":2}`), "type"); err != nil || got != "a" {
		t.Fatalf("got %q, %v", got, err)
	}

	for in, contains := range map[string]string{
		`[1]`:         "want an object",
		`{"type":1}`:  `member "type" is not a string`,
		`{"x":1}`:     `missing object member name "type"`,
		`{"x":`:       "EOF",
		`{"type":`:    "EOF",
		`{"x":1,"y"`:  "EOF",
		``:            "EOF",
		`{"x":[1,2,]`: "invalid",
	} {
		_, err := union.MemberString(jsontext.Value(in), "type")
		wantErr(t, err, contains)
	}
}

func TestMembers(t *testing.T) {
	if got, err := union.Members(jsontext.Value(`{"b":1,"a":{"c":2}}`)); err != nil || !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("got %q, %v", got, err)
	}

	for in, contains := range map[string]string{`[1]`: "want an object", `{"a":`: "EOF", ``: "EOF", `{"a"`: "EOF"} {
		_, err := union.Members(jsontext.Value(in))
		wantErr(t, err, contains)
	}
}

func TestSelect(t *testing.T) {
	got, err := union.Select(jsontext.Value(`{"a":1,"b":[2],"c":3}`), map[string]bool{"a": true, "c": true})
	if err != nil || string(got) != `{"a":1,"c":3}` {
		t.Fatalf("got %s, %v", got, err)
	}

	for _, in := range []string{``, `{"a":`, `{"a"`} {
		if _, err := union.Select(jsontext.Value(in), nil); err == nil {
			t.Errorf("%q: got no error", in)
		}
	}
}

func TestMerge(t *testing.T) {
	got, err := union.Merge(jsontext.Value(`{"a":1,"b":{"x":1,"y":2}}`), jsontext.Value(`{"b":{"y":2,"x":1},"c":3}`))
	if err != nil || string(got) != `{"a":1,"b":{"x":1,"y":2},"c":3}` {
		t.Fatalf("got %s, %v", got, err)
	}

	_, err = union.Merge(jsontext.Value(`{"a":1}`), jsontext.Value(`{"a":2}`))
	wantErr(t, err, `member "a" is 1 in one part and 2 in another`)

	for _, tc := range [][2]string{{``, `{}`}, {`{}`, `{"a":`}, {`{"a"`, `{}`}} {
		if _, err := union.Merge(jsontext.Value(tc[0]), jsontext.Value(tc[1])); err == nil {
			t.Errorf("%q and %q: got no error", tc[0], tc[1])
		}
	}
}

func TestEnumsFit(t *testing.T) {
	enums := map[string][]string{"color": {`"red"`, `"blue"`}}

	for raw, want := range map[string]bool{
		`{"color":"red","x":1}`: true,
		`{"color": "blue"}`:     true,
		`{"x":1}`:               true,
		`{"color":"green"}`:     false,
		`[1]`:                   false,
		`{"color"`:              false,
		`{"x":`:                 false,
	} {
		if got := union.EnumsFit(jsontext.Value(raw), enums); got != want {
			t.Errorf("%s: got %v, want %v", raw, got, want)
		}
	}

	if !union.EnumsFit(jsontext.Value(`"anything"`), nil) {
		t.Error("no enums: does not fit")
	}
}

func TestEnumPasses(t *testing.T) {
	if got := union.EnumPasses(true); !slices.Equal(got, []bool{true}) {
		t.Errorf("strict: got %v", got)
	}

	if got := union.EnumPasses(false); !slices.Equal(got, []bool{true, false}) {
		t.Errorf("lenient: got %v", got)
	}
}

func TestFits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		required []string
		nullable map[string]bool
		pinned   map[string]string
		want     bool
	}{
		{"nothing asked", `"any"`, nil, nil, nil, true},
		{"not an object", `[1]`, []string{"a"}, nil, nil, false},
		{"required there", `{"a":1,"b":2}`, []string{"a"}, nil, nil, true},
		{"required missing", `{"b":2}`, []string{"a"}, nil, nil, false},
		{"required null", `{"a":null}`, []string{"a"}, nil, nil, false},
		{"required null, and it may be", `{"a":null}`, []string{"a"}, map[string]bool{"a": true}, nil, true},
		{"pinned to its value", `{"kind": "x"}`, nil, nil, map[string]string{"kind": `"x"`}, true},
		{"pinned to another", `{"kind":"y"}`, nil, nil, map[string]string{"kind": `"x"`}, false},
		{"pinned, left out", `{"a":1}`, nil, nil, map[string]string{"kind": `"x"`}, true},
		{"broken", `{"a":`, []string{"a"}, nil, nil, false},
		{"broken name", `{"a"`, []string{"a"}, nil, nil, false},
	} {
		if got := union.Fits(jsontext.Value(tc.raw), tc.required, tc.nullable, tc.pinned); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChooseVariants(t *testing.T) {
	cat := union.Variant{Value: "cat", Members: map[string]bool{"kind": true, "meow": true}, Required: []string{"meow"}}
	dog := union.Variant{Value: "dog", Members: map[string]bool{"kind": true, "bark": true}, Required: []string{"bark"}}
	pet := union.Variant{Value: "pet", Members: map[string]bool{"kind": true, "meow": true, "bark": true}}
	red := union.Variant{Value: "red", Members: map[string]bool{"color": true}, Enums: map[string][]string{"color": {`"red"`}}}
	blue := union.Variant{Value: "blue", Members: map[string]bool{"color": true}, Enums: map[string][]string{"color": {`"blue"`}}}
	plain := map[string]bool{"id": true}

	for _, tc := range []struct {
		name          string
		raw           string
		discriminator string
		variants      []union.Variant
		oneOf, strict bool
		want          []int
	}{
		{"by the discriminator", `{"kind":"dog","bark":true}`, "kind", []union.Variant{cat, dog}, true, true, []int{1}},
		{"by the members required", `{"meow":true,"id":1}`, "", []union.Variant{cat, dog}, true, true, []int{0}},
		{"oneOf, leniently the first of several", `{"meow":true}`, "", []union.Variant{cat, pet}, true, false, []int{0}},
		{"anyOf, all that fit", `{"meow":true}`, "", []union.Variant{cat, pet}, false, true, []int{0, 1}},
		{"by its enum", `{"color":"blue"}`, "", []union.Variant{red, blue}, true, true, []int{1}},
		{"leniently, outside every enum", `{"color":"green"}`, "", []union.Variant{red, blue}, true, false, []int{0}},
		{"leniently, a member nothing declares", `{"meow":true,"new":1}`, "", []union.Variant{cat, dog}, true, false, []int{0}},
	} {
		got, err := union.ChooseVariants(jsontext.Value(tc.raw), tc.discriminator, tc.variants, plain, tc.oneOf, tc.strict)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, %v, want %v", tc.name, got, err, tc.want)
		}
	}

	for _, tc := range []struct {
		name, raw, discriminator string
		variants                 []union.Variant
		oneOf, strict            bool
		contains                 string
	}{
		{"a value of the discriminator none has", `{"kind":"cow"}`, "kind", []union.Variant{cat, dog}, true, true, `unknown value of "kind"`},
		{"without the discriminator", `{"meow":true}`, "kind", []union.Variant{cat, dog}, true, true, `missing object member name "kind"`},
		{"none fits", `{"moo":true}`, "", []union.Variant{cat, dog}, true, false, "matches none of its alternatives"},
		{"oneOf, strict, several fit", `{"meow":true}`, "", []union.Variant{cat, pet}, true, true, "matches 2 of its alternatives, want exactly one"},
		{"strict, outside every enum", `{"color":"green"}`, "", []union.Variant{red, blue}, true, true, "matches none of its alternatives"},
		{"strict, a member the chosen does not declare", `{"kind":"cat","meow":true,"bark":true}`, "kind", []union.Variant{cat, dog}, true, true, `"bark"`},
		{"not an object", `[1]`, "", []union.Variant{cat}, true, true, "want an object"},
	} {
		_, err := union.ChooseVariants(jsontext.Value(tc.raw), tc.discriminator, tc.variants, plain, tc.oneOf, tc.strict)
		if err == nil || !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("%s: got %v, want an error with %q", tc.name, err, tc.contains)
		}
	}
}

func TestCheckTag(t *testing.T) {
	tags := map[string]map[string]union.TagNeed{
		"text":  {"text": union.TagRequired, "color": union.TagOptional},
		"image": {"image": union.TagRequiredOrNull},
	}

	for _, tc := range []struct {
		name         string
		tag          string
		set, present []string
		strict       bool
		contains     string // empty for no error
	}{
		{"decoded, all there", "text", []string{"text"}, []string{"type", "text"}, true, ""},
		{"decoded, a member that may be null, null", "image", nil, []string{"type", "image"}, true, ""},
		{"decoded, a required member missing", "text", nil, []string{"type"}, true, `missing object member name "text"`},
		{"decoded, a member that may be null, missing", "image", nil, []string{"type"}, true, `missing object member name "image"`},
		{"encoded, all there", "text", []string{"text", "color"}, nil, true, ""},
		{"encoded, a member that may be null, left out", "image", nil, nil, true, ""},
		{"encoded, a required member left out", "text", []string{"color"}, nil, true, `missing object member name "text"`},
		{"strict, another alternative's member", "text", []string{"text", "image"}, nil, true, `type "text" does not allow member "image"`},
		{"leniently, another alternative's member", "text", []string{"text", "image"}, nil, false, ""},
		{"no tag", "", nil, nil, true, `missing object member name "type"`},
		{"a tag none has", "video", nil, nil, true, `unknown value of "type"`},
	} {
		err := union.CheckTag("type", tc.tag, tags, tc.set, tc.present, tc.strict)
		switch {
		case tc.contains == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.contains != "" && (err == nil || !strings.Contains(err.Error(), tc.contains)):
			t.Errorf("%s: got %v, want an error with %q", tc.name, err, tc.contains)
		}
	}
}

func TestInferTag(t *testing.T) {
	tags := map[string]map[string]union.TagNeed{
		"text":  {"text": union.TagRequired, "color": union.TagOptional},
		"image": {"image": union.TagRequired},
		"color": {"color": union.TagRequired},
	}

	for _, tc := range []struct {
		set  []string
		want string
	}{
		{[]string{"image"}, "image"},
		{[]string{"text", "color"}, "text"},
		{[]string{"text", "image"}, ""},
		{[]string{"color"}, "color"},
		{nil, ""},
		{[]string{"other"}, ""},
	} {
		got, ok := union.InferTag(tags, tc.set)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%q: got %q, %v, want %q", tc.set, got, ok, tc.want)
		}
	}
}
