package climux

import (
	"context"
	"fmt"
	"net"
)

// ipDecoder decodes a net.IP.
type ipDecoder struct{}

func (ipDecoder) Decode(v *net.IP, s string) error {
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", s)
	}
	*v = ip
	return nil
}

// IPVar returns a Flag that can be used to define a net.IP flag with
// specified name, default value, and usage string. The argument p points to a
// net.IP variable in which to store the value of the flag.
func IPVar(p *net.IP, name string, value net.IP, usage string) *Flag {
	*p = value
	return Var(p, name, usage, ipDecoder{})
}

func ExampleDecoder() {
	var ip net.IP

	cmd := NewCommand("ping", "").
		Flags(
			// configure a net.IP flag with our custom Decoder
			IPVar(&ip, "ip", net.IPv6zero, "IP address to ping"),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "ping: %s\n", ip)
			return nil
		})

	Run(context.Background(), cmd, WithArgs("--ip=ff02:0000:0000:0000:0000:0000:0000:0001"))
	// Output: ping: ff02::1
}
