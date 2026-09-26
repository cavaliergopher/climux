package climux

import (
	"context"
	"fmt"
	"net"

	"go.hotsrc.dev/climux/ir"
)

// ipType describes a net.IP to climux: how one is decoded, shown and
// classified.
type ipType struct{}

func (ipType) Decode(v *net.IP, s string) error {
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", s)
	}
	*v = ip
	return nil
}

func (ipType) Format(v net.IP) string { return v.String() }
func (ipType) Kind() ir.Kind          { return ir.KindOpaque }

// IPVar returns a net.IP flag with the specified name and usage string.
// The argument p points to a net.IP variable in which to store the value
// of the flag.
func IPVar(p *net.IP, name, usage string) *FlagBuilder[net.IP] {
	return Var(p, name, usage, ipType{})
}

func ExampleVarType() {
	var ip net.IP

	cmd := NewCommand("ping", "").
		Flags(
			// configure a net.IP flag with our custom VarType
			IPVar(&ip, "ip", "IP address to ping").Default(net.IPv6zero),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "ping: %s\n", ip)
			return nil
		})

	Run(context.Background(), cmd, WithArgs("--ip=ff02:0000:0000:0000:0000:0000:0000:0001"))
	// Output: ping: ff02::1
}
