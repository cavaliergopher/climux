package climux

import (
	"strconv"
	"time"

	"go.hotsrc.dev/climux/ir"
)

// Decoder turns one option-argument into a T, written over what earlier
// namings of the same flag left in value. On the flag's first naming
// *value is the zero T, because a flag's first naming replaces its
// default: a scalar overwrites, and a slice or map initialises there.
//
// It is what Var takes, and the one thing this package asks an author
// to write for a type it has no constructor for:
//
//	type ipDecoder struct{}
//
//	func (ipDecoder) Decode(v *net.IP, s string) error {
//		ip := net.ParseIP(s)
//		if ip == nil {
//			return fmt.Errorf("invalid IP: %s", s)
//		}
//		*v = ip
//		return nil
//	}
//
// A Decoder may add two optional methods, named as Go's flag package
// names them. Kind() ir.Kind says what kind of value it decodes, which
// is otherwise ir.KindOpaque. IsBoolFlag() bool, reporting true, lets the
// flag stand alone on the command line the way a boolean does.
type Decoder[T any] interface {
	Decode(value *T, s string) error
}

// DecodeFunc adapts a function to Decoder, as http.HandlerFunc adapts one
// to http.Handler.
type DecodeFunc[T any] func(value *T, s string) error

// Decode calls f.
func (f DecodeFunc[T]) Decode(value *T, s string) error { return f(value, s) }

// value adapts a Decoder and the variable it writes to ir.Value, which
// is what the parser calls.
type value[T any] struct {
	p   *T
	dec Decoder[T]

	// named records that the flag has been named once, so that the
	// first naming finds the zero T rather than the default.
	named bool
}

// Set zeroes the variable on the flag's first naming and decodes s into
// it.
func (v *value[T]) Set(s string) error {
	if !v.named {
		var zero T
		*v.p = zero
		v.named = true
	}
	return v.dec.Decode(v.p, s)
}

// Kind and IsBoolFlag answer for the decoder, which may or may not have
// an opinion. See Decoder.
func (v *value[T]) Kind() ir.Kind {
	if k, ok := v.dec.(interface{ Kind() ir.Kind }); ok {
		return k.Kind()
	}
	return ir.KindOpaque
}

func (v *value[T]) IsBoolFlag() bool {
	b, ok := v.dec.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// isBoolValue reports whether v declares itself a boolean flag by
// implementing ir.BoolValue, which is what lets it stand alone on the
// command line without an attached value.
func isBoolValue(v ir.Value) bool {
	if bv, ok := v.(ir.BoolValue); ok {
		return bv.IsBoolFlag()
	}
	return false
}

// kindOf returns the ir.Kind v declares for itself by implementing
// ir.KindValue, or ir.KindOpaque when it does not.
func kindOf(v ir.Value) ir.Kind {
	if kv, ok := v.(ir.KindValue); ok {
		return kv.Kind()
	}
	return ir.KindOpaque
}

// boolDecoder decodes a bool and lets its flag stand alone.
type boolDecoder struct{}

func (boolDecoder) IsBoolFlag() bool { return true }

func (boolDecoder) Decode(v *bool, s string) error {
	b, err := strconv.ParseBool(s)
	*v = b
	return err
}

// bitFieldDecoder decodes a bool and, when it is true, sets mask in the
// word several flags share. Nothing clears a bit: a false leaves the
// word alone.
type bitFieldDecoder struct {
	word *uint64
	mask uint64
}

func (bitFieldDecoder) IsBoolFlag() bool { return true }

func (d bitFieldDecoder) Decode(v *bool, s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*v = b
	if b {
		*d.word |= d.mask
	}
	return nil
}

func decodeDuration(v *time.Duration, s string) error {
	d, err := time.ParseDuration(s)
	*v = d
	return err
}

func decodeFloat64(v *float64, s string) error {
	f, err := strconv.ParseFloat(s, 64)
	*v = f
	return err
}

func decodeInt(v *int, s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	*v = int(n)
	return err
}

func decodeInt64(v *int64, s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	*v = n
	return err
}

func decodeString(v *string, s string) error {
	*v = s
	return nil
}

func decodeStrings(v *[]string, s string) error {
	*v = append(*v, s)
	return nil
}

func decodeUint(v *uint, s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	*v = uint(n)
	return err
}

func decodeUint64(v *uint64, s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	*v = n
	return err
}
