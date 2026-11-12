// Package union holds what the code openapi-codegen generates decodes and encodes unions and tagged structs with:
// JSON values of several alternatives, of which the code chooses one by a discriminator, by the members an
// alternative requires and declares, or by a tag. It is generated code's, not an API of its own, and changes as the
// generator needs.
package union

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// Strict reports whether dec rejects members the specification does not know, as it does unless told otherwise.
func Strict(dec *jsontext.Decoder) bool {
	v, ok := json.GetOption(dec.Options(), json.RejectUnknownMembers)
	return v || !ok
}

// UnknownName reports a member no part of the type declares, as encoding/json reports one of a struct.
func UnknownName(name string) error {
	return &json.SemanticError{JSONKind: jsontext.KindString, Err: fmt.Errorf("%w %q", json.ErrUnknownName, name)}
}

// Missing reports a member the type needs and the object lacks.
func Missing(name string) error {
	return &json.SemanticError{JSONKind: jsontext.KindBeginObject, Err: fmt.Errorf("missing object member name %q", name)}
}

// UnknownValue reports a value of the discriminator that names no alternative.
func UnknownValue(name, value string) error {
	return &json.SemanticError{JSONKind: jsontext.KindString, JSONValue: jsontext.Value(strconv.Quote(value)), Err: fmt.Errorf("unknown value of %q", name)}
}

// FirstMember reads the opening of a JSON object and its member name, which must have a string value. It returns
// the string, the value as written, and the decoder to read the object's further members from: dec itself when name
// comes first, the fast way, and otherwise one over the object read whole, with name moved to the front.
func FirstMember(dec *jsontext.Decoder, name string) (string, jsontext.Value, *jsontext.Decoder, error) {
	if tok, err := dec.ReadToken(); err != nil {
		return "", nil, nil, err
	} else if tok.Kind() != jsontext.KindBeginObject {
		return "", nil, nil, &json.SemanticError{JSONKind: tok.Kind(), Err: errors.New("want an object")}
	}

	if dec.PeekKind() == jsontext.KindEndObject {
		return "", nil, nil, Missing(name)
	}

	tok, err := dec.ReadToken()
	if err != nil {
		return "", nil, nil, err
	}

	if got := tok.String(); got != name {
		// read the rest of the object, and start again with name first
		var buf bytes.Buffer

		enc := jsontext.NewEncoder(&buf)
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return "", nil, nil, err
		}

		if err := enc.WriteToken(jsontext.String(got)); err != nil {
			return "", nil, nil, err
		}

		for dec.PeekKind() != jsontext.KindEndObject {
			val, err := dec.ReadValue()
			if err != nil {
				return "", nil, nil, err
			}

			if err := enc.WriteValue(val); err != nil {
				return "", nil, nil, err
			}

			if dec.PeekKind() == jsontext.KindEndObject {
				break
			}

			tok, err := dec.ReadToken()
			if err != nil {
				return "", nil, nil, err
			}

			if err := enc.WriteToken(tok); err != nil {
				return "", nil, nil, err
			}
		}

		if _, err := dec.ReadToken(); err != nil { // the end of the object
			return "", nil, nil, err
		}

		if err := enc.WriteToken(jsontext.EndObject); err != nil {
			return "", nil, nil, err
		}

		ordered, err := First(buf.Bytes(), name, "")
		if err != nil {
			return "", nil, nil, err
		}

		return FirstMember(jsontext.NewDecoder(bytes.NewReader(ordered), dec.Options()), name)
	}

	val, err := dec.ReadValue()
	if err != nil {
		return "", nil, nil, err
	}

	var tag string
	if err := json.Unmarshal(val, &tag); err != nil {
		return "", nil, nil, &json.SemanticError{JSONKind: val.Kind(), Err: fmt.Errorf("member %q is not a string", name)}
	}

	return tag, val.Clone(), dec, nil
}

// MembersFrom decodes the first member, already read, and every further member of the object dec is in, each
// into the field decode declares for it; a member it does not declare is an error, unless dec is lenient.
func MembersFrom(dec *jsontext.Decoder, firstName string, first jsontext.Value, decode func(*jsontext.Decoder, string) (bool, error)) error {
	strict := Strict(dec)

	if ok, err := decode(jsontext.NewDecoder(bytes.NewReader(first), dec.Options()), firstName); err != nil {
		return err
	} else if !ok && strict {
		return UnknownName(firstName)
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}

		name := tok.String() // a token is void once the decoder reads on

		if ok, err := decode(dec, name); err != nil {
			return err
		} else if !ok && strict {
			return UnknownName(name)
		} else if !ok {
			if err := dec.SkipValue(); err != nil {
				return err
			}
		}
	}

	_, err := dec.ReadToken()

	return err
}

// Part is one of the parts an object decodes into: the members it declares and how it decodes one of them.
type Part struct {
	Members map[string]bool
	Decode  func(*jsontext.Decoder, string) (bool, error)
}

// PartsFrom is MembersFrom for an object that several parts declare members of. A member one part declares
// decodes straight from dec; one several declare is read once and decoded into each.
func PartsFrom(dec *jsontext.Decoder, firstName string, first jsontext.Value, parts []Part) error {
	member := func(name string, val jsontext.Value) error {
		var declaring []Part
		for _, p := range parts {
			if p.Members[name] {
				declaring = append(declaring, p)
			}
		}

		switch {
		case len(declaring) > 0:
		case Strict(dec):
			return UnknownName(name)
		case val == nil:
			return dec.SkipValue()
		default:
			return nil
		}

		if val == nil && len(declaring) == 1 {
			_, err := declaring[0].Decode(dec, name)
			return err
		}

		if val == nil {
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}

			val = v.Clone()
		}

		for _, p := range declaring {
			if _, err := p.Decode(jsontext.NewDecoder(bytes.NewReader(val), dec.Options()), name); err != nil {
				return err
			}
		}

		return nil
	}

	if err := member(firstName, first); err != nil {
		return err
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}

		if err := member(tok.String(), nil); err != nil {
			return err
		}
	}

	_, err := dec.ReadToken()

	return err
}

// First returns the JSON object raw with its member name first, as decoding by a discriminator wants it. If value
// is known, the member must have it, and is written in if raw lacks it or leaves it empty. raw is returned as it is if
// all is in order.
func First(raw jsontext.Value, name, value string) (jsontext.Value, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}

	if dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}

		if tok.String() == name {
			if value == "" {
				return raw, nil
			}

			val, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}

			switch got := val.String(); got {
			case value:
				return raw, nil
			case "": // left for the union to fill in, below
			default:
				return nil, &json.SemanticError{Err: fmt.Errorf("member %q is %q, want %q", name, got, value)}
			}
		}
	}

	names, err := Members(raw)
	if err != nil {
		return nil, err
	}

	var first jsontext.Value
	switch {
	case slices.Contains(names, name):
		if first, err = Select(raw, map[string]bool{name: true}); err != nil {
			return nil, err
		}

		if value == "" {
			break
		}

		got, err := MemberString(first, name)
		switch {
		case err != nil:
			return nil, err
		case got == "": // left empty: the union knows what it holds
			rest := make(map[string]bool, len(names))
			for _, n := range names {
				rest[n] = n != name
			}

			if raw, err = Select(raw, rest); err != nil {
				return nil, err
			}

			if first, err = json.Marshal(map[string]string{name: value}); err != nil {
				return nil, err
			}
		case got != value:
			return nil, &json.SemanticError{Err: fmt.Errorf("member %q is %q, want %q", name, got, value)}
		}
	case value != "":
		member, err := json.Marshal(map[string]string{name: value})
		if err != nil {
			return nil, err
		}

		first = member
	default:
		return nil, Missing(name)
	}

	return Merge(first, raw)
}

// MemberString returns the string value of the member name of the JSON object raw, reading no further than it.
func MemberString(raw jsontext.Value, name string) (string, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.ReadToken(); err != nil {
		return "", err
	} else if tok.Kind() != jsontext.KindBeginObject {
		return "", &json.SemanticError{JSONKind: tok.Kind(), Err: errors.New("want an object")}
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return "", err
		}

		if tok.String() != name {
			if err := dec.SkipValue(); err != nil {
				return "", err
			}

			continue
		}

		val, err := dec.ReadToken()
		if err != nil {
			return "", err
		}

		if val.Kind() != jsontext.KindString {
			return "", &json.SemanticError{JSONKind: val.Kind(), Err: fmt.Errorf("member %q is not a string", name)}
		}

		return val.String(), nil
	}

	return "", Missing(name)
}

// Members returns the names of the members of the JSON object raw, in order.
func Members(raw jsontext.Value) ([]string, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.ReadToken(); err != nil {
		return nil, err
	} else if tok.Kind() != jsontext.KindBeginObject {
		return nil, &json.SemanticError{JSONKind: tok.Kind(), Err: errors.New("want an object")}
	}

	var names []string
	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}

		names = append(names, tok.String())

		if err := dec.SkipValue(); err != nil {
			return nil, err
		}
	}

	return names, nil
}

// Select returns the JSON object raw with only the members keep holds.
func Select(raw jsontext.Value, keep map[string]bool) (jsontext.Value, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	enc := jsontext.NewEncoder(&buf)
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return nil, err
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}

		name := tok.String() // a token is void once the decoder reads on

		val, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}

		if !keep[name] {
			continue
		}

		if err := enc.WriteToken(jsontext.String(name)); err != nil {
			return nil, err
		}

		if err := enc.WriteValue(val); err != nil {
			return nil, err
		}
	}

	if err := enc.WriteToken(jsontext.EndObject); err != nil {
		return nil, err
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

// Merge returns the members of the JSON objects a and b in one object. A member both hold must have the same
// value in each.
func Merge(a, b jsontext.Value) (jsontext.Value, error) {
	var buf bytes.Buffer

	enc := jsontext.NewEncoder(&buf)
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return nil, err
	}

	written := map[string]jsontext.Value{}
	for _, raw := range []jsontext.Value{a, b} {
		dec := jsontext.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.ReadToken(); err != nil {
			return nil, err
		}

		for dec.PeekKind() != jsontext.KindEndObject {
			tok, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}

			name := tok.String() // a token is void once the decoder reads on

			val, err := dec.ReadValue()
			if err != nil {
				return nil, err
			}

			if prev, ok := written[name]; ok {
				if err := prev.Canonicalize(); err != nil {
					return nil, err
				}

				cur := val.Clone()
				if err := cur.Canonicalize(); err != nil {
					return nil, err
				}

				if !bytes.Equal(prev, cur) {
					return nil, &json.SemanticError{Err: fmt.Errorf("member %q is %s in one part and %s in another", name, prev, cur)}
				}

				continue
			}

			written[name] = val.Clone()

			if err := enc.WriteToken(jsontext.String(name)); err != nil {
				return nil, err
			}

			if err := enc.WriteValue(val); err != nil {
				return nil, err
			}
		}
	}

	if err := enc.WriteToken(jsontext.EndObject); err != nil {
		return nil, err
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

// Variant is an alternative of a union: its value of the discriminator, the members it declares and those it
// requires.
type Variant struct {
	Value    string
	Members  map[string]bool
	Required []string
	// Nullable are the members of Required that may be null.
	Nullable map[string]bool
	// Pinned are the members it allows one value for, written as compact JSON.
	Pinned map[string]string
	// Enums are the members it allows the values of an enum for, written so.
	Enums map[string][]string
}

// EnumsFit reports whether each member of the JSON object raw that enums names has one of the values it allows,
// written as compact JSON.
func EnumsFit(raw jsontext.Value, enums map[string][]string) bool {
	if len(enums) == 0 {
		return true
	}

	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != jsontext.KindBeginObject {
		return false
	}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return false
		}

		name := tok.String()

		val, err := dec.ReadValue()
		if err != nil {
			return false
		}

		if allowed, ok := enums[name]; ok {
			got := val.Clone()
			if err := got.Compact(); err != nil || !slices.Contains(allowed, string(got)) {
				return false
			}
		}
	}

	return true
}

// EnumPasses are whether to check enums in each attempt at choosing an alternative: strict, always; leniently,
// first, then not, should no alternative fit, as an API may add a value to an enum.
func EnumPasses(strict bool) []bool {
	if strict {
		return []bool{true}
	}

	return []bool{true, false}
}

// Fits reports whether the JSON value raw has the members required, and those of pinned it has with the value
// each pins, written as compact JSON. A member that is null is there only where nullable allows it to be. Any value
// fits where nothing is required or pinned.
func Fits(raw jsontext.Value, required []string, nullable map[string]bool, pinned map[string]string) bool {
	if len(required) == 0 && len(pinned) == 0 {
		return true
	}

	if raw.Kind() != jsontext.KindBeginObject {
		return false
	}

	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.ReadToken(); err != nil {
		return false
	}

	present := map[string]bool{}

	for dec.PeekKind() != jsontext.KindEndObject {
		tok, err := dec.ReadToken()
		if err != nil {
			return false
		}

		name := tok.String()

		val, err := dec.ReadValue()
		if err != nil {
			return false
		}

		present[name] = val.Kind() != jsontext.KindNull || nullable[name]

		if want, ok := pinned[name]; ok {
			got := val.Clone()
			if err := got.Compact(); err != nil || string(got) != want {
				return false
			}
		}
	}

	for _, r := range required {
		if !present[r] {
			return false
		}
	}

	return true
}

// ChooseVariants returns the alternatives the JSON object raw is: the one its discriminator names, if there is
// one, else those whose required members it has and whose members it holds, besides those of plain. Every member
// must belong to plain or to a chosen alternative. Not strict, a member nothing declares is left out of the choice,
// and of several alternatives of a oneOf the first is chosen.
func ChooseVariants(raw jsontext.Value, discriminator string, variants []Variant, plain map[string]bool, oneOf, strict bool) ([]int, error) {
	names, err := Members(raw)
	if err != nil {
		return nil, err
	}

	if !strict {
		names = slices.DeleteFunc(names, func(n string) bool {
			return !plain[n] && !slices.ContainsFunc(variants, func(v Variant) bool { return v.Members[n] })
		})
	}

	present := make(map[string]bool, len(names))
	for _, n := range names {
		present[n] = true
	}

	var chosen []int
	if discriminator != "" {
		tag, err := MemberString(raw, discriminator)
		if err != nil {
			return nil, err
		}

		for i, v := range variants {
			if v.Value == tag {
				chosen = append(chosen, i)
			}
		}

		if len(chosen) == 0 {
			return nil, UnknownValue(discriminator, tag)
		}
	} else {
	variants:
		for i, v := range variants {
			if !Fits(raw, v.Required, v.Nullable, v.Pinned) {
				continue variants
			}

			for _, n := range names {
				if !plain[n] && !v.Members[n] {
					continue variants
				}
			}

			chosen = append(chosen, i)
		}

		// a member outside its enum rules an alternative out; leniently, only while another fits
		fit := slices.DeleteFunc(slices.Clone(chosen), func(i int) bool { return !EnumsFit(raw, variants[i].Enums) })
		if len(fit) > 0 || strict {
			chosen = fit
		}

		switch {
		case len(chosen) == 0:
			return nil, &json.SemanticError{Err: errors.New("matches none of its alternatives")}
		case oneOf && len(chosen) > 1 && strict:
			return nil, &json.SemanticError{Err: fmt.Errorf("matches %d of its alternatives, want exactly one", len(chosen))}
		case oneOf:
			chosen = chosen[:1]
		}
	}

	for _, n := range names {
		if plain[n] {
			continue
		}

		known := false
		for _, i := range chosen {
			known = known || variants[i].Members[n]
		}

		if !known {
			return nil, UnknownName(n)
		}
	}

	return chosen, nil
}

// TagNeed is how an alternative needs a member of its own.
type TagNeed int

const (
	TagOptional TagNeed = iota
	TagRequired
	TagRequiredOrNull
)

// CheckTag reports an error unless set, the members of an alternative's own whose fields are set, holds only
// members of the alternative the value tag of the tag name names, and the members it requires are there: in present,
// the members of the object decoded, null ones included, or, encoding, with present nil, in set, where a member that
// may be null cannot be told from one left out. Not strict, set may hold others too.
func CheckTag(name, tag string, tags map[string]map[string]TagNeed, set, present []string, strict bool) error {
	members, ok := tags[tag]
	switch {
	case tag == "":
		return Missing(name)
	case !ok:
		return UnknownValue(name, tag)
	}

	for _, s := range set {
		if _, ok := members[s]; strict && !ok {
			return &json.SemanticError{JSONKind: jsontext.KindBeginObject, Err: fmt.Errorf("%s %q does not allow member %q", name, tag, s)}
		}
	}

	for _, m := range slices.Sorted(maps.Keys(members)) {
		switch need := members[m]; {
		case present != nil && need != TagOptional && !slices.Contains(present, m),
			present == nil && need == TagRequired && !slices.Contains(set, m):
			return Missing(m)
		}
	}

	return nil
}

// InferTag returns the value of a tag whose alternative is the one of tags that set, the members of an
// alternative's own that are set, names: the one set member is a value whose alternative allows all of set.
func InferTag(tags map[string]map[string]TagNeed, set []string) (string, bool) {
	tag := ""

	for _, s := range set {
		members, ok := tags[s]
		if !ok || !allIn(set, members) {
			continue
		}

		if tag != "" {
			return "", false
		}

		tag = s
	}

	return tag, tag != ""
}

// allIn reports whether every one of names is a key of m.
func allIn(names []string, m map[string]TagNeed) bool {
	for _, n := range names {
		if _, ok := m[n]; !ok {
			return false
		}
	}

	return true
}
