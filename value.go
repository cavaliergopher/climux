package climux

import (
	"fmt"
	"strconv"
	"time"

	"go.hotsrc.dev/climux/ir"
)

// VarType describes a type this package has no constructor for: how to
// decode one from an option-argument, how to show one in help, and what
// kind of value the schema reports. It is what Var takes, and the one
// thing an author writes to bind a variable of their own type:
//
//	type ipType struct{}
//
//	func (ipType) Decode(v *net.IP, s string) error {
//		ip := net.ParseIP(s)
//		if ip == nil {
//			return fmt.Errorf("invalid IP: %s", s)
//		}
//		*v = ip
//		return nil
//	}
//
//	func (ipType) Format(v net.IP) string { return v.String() }
//	func (ipType) Kind() ir.Kind         { return ir.KindOpaque }
//
// Decode writes over what earlier namings of the same flag left in
// value. On the flag's first naming *value is the zero T, because a
// flag's first naming replaces its default: a scalar overwrites, and a
// slice or map initialises there. A VarType carries no state of its
// own; the variable is the state.
//
// Format renders a value as help shows it, which is how a default is
// displayed. Kind is what the schema reports for the flag; see ir.Kind.
//
// A VarType may add IsBoolFlag() bool, named as Go's flag package names
// it: reporting true lets the flag stand alone on the command line the
// way a boolean does.
type VarType[T any] interface {
	Decode(value *T, s string) error
	Format(value T) string
	Kind() ir.Kind
}

// value adapts a FlagState to ir.Value, which is what the parser calls:
// Set hands the flag's variable to its VarType.
type value[T any] struct{ s *FlagState[T] }

// Set zeroes the variable on the flag's first naming and decodes s into
// it. The parser records the naming after Set returns, so a zero Count
// here means this is the first, and the default the variable may hold
// is discarded before the type sees it.
func (v value[T]) Set(s string) error {
	st := v.s
	if st.shared.Count == 0 {
		var zero T
		*st.p = zero
	}
	return st.owner.t.Decode(st.p, s)
}

// Kind and IsBoolFlag answer for the type. See VarType.
func (v value[T]) Kind() ir.Kind { return v.s.owner.t.Kind() }

func (v value[T]) IsBoolFlag() bool {
	b, ok := v.s.owner.t.(interface{ IsBoolFlag() bool })
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

// The types behind the typed constructors.

type boolType struct{}

func (boolType) IsBoolFlag() bool     { return true }
func (boolType) Kind() ir.Kind        { return ir.KindBool }
func (boolType) Format(v bool) string { return strconv.FormatBool(v) }
func (boolType) Decode(v *bool, s string) error {
	b, err := strconv.ParseBool(s)
	*v = b
	return err
}

// bitFieldType decodes a bool and, when it is true, sets mask in the
// word several flags share. Nothing clears a bit: a false leaves the
// word alone.
type bitFieldType struct {
	word *uint64
	mask uint64
}

func (bitFieldType) IsBoolFlag() bool     { return true }
func (bitFieldType) Kind() ir.Kind        { return ir.KindBool }
func (bitFieldType) Format(v bool) string { return strconv.FormatBool(v) }
func (t bitFieldType) Decode(v *bool, s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*v = b
	if b {
		*t.word |= t.mask
	}
	return nil
}

type durationType struct{}

func (durationType) Kind() ir.Kind                 { return ir.KindDuration }
func (durationType) Format(v time.Duration) string { return v.String() }
func (durationType) Decode(v *time.Duration, s string) error {
	d, err := time.ParseDuration(s)
	*v = d
	return err
}

type float64Type struct{}

func (float64Type) Kind() ir.Kind           { return ir.KindFloat }
func (float64Type) Format(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
func (float64Type) Decode(v *float64, s string) error {
	f, err := strconv.ParseFloat(s, 64)
	*v = f
	return err
}

// funcType hands each argument to the function Func was given, and
// records only that the flag was named. It is the anonymous case, so it
// claims no kind and shows no default.
type funcType func(s string) error

func (funcType) Kind() ir.Kind      { return ir.KindOpaque }
func (funcType) Format(bool) string { return "" }
func (f funcType) Decode(v *bool, s string) error {
	*v = true
	return f(s)
}

type intType struct{}

func (intType) Kind() ir.Kind       { return ir.KindInt }
func (intType) Format(v int) string { return strconv.Itoa(v) }
func (intType) Decode(v *int, s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	*v = int(n)
	return err
}

type int64Type struct{}

func (int64Type) Kind() ir.Kind         { return ir.KindInt }
func (int64Type) Format(v int64) string { return strconv.FormatInt(v, 10) }
func (int64Type) Decode(v *int64, s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	*v = n
	return err
}

type stringType struct{}

func (stringType) Kind() ir.Kind          { return ir.KindString }
func (stringType) Format(v string) string { return v }
func (stringType) Decode(v *string, s string) error {
	*v = s
	return nil
}

type stringsType struct{}

func (stringsType) Kind() ir.Kind            { return ir.KindString }
func (stringsType) Format(v []string) string { return fmt.Sprint(v) }
func (stringsType) Decode(v *[]string, s string) error {
	*v = append(*v, s)
	return nil
}

type uintType struct{}

func (uintType) Kind() ir.Kind        { return ir.KindUint }
func (uintType) Format(v uint) string { return strconv.FormatUint(uint64(v), 10) }
func (uintType) Decode(v *uint, s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	*v = uint(n)
	return err
}

type uint64Type struct{}

func (uint64Type) Kind() ir.Kind          { return ir.KindUint }
func (uint64Type) Format(v uint64) string { return strconv.FormatUint(v, 10) }
func (uint64Type) Decode(v *uint64, s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	*v = n
	return err
}
