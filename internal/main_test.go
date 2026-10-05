package internal

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
)

// TestMain keeps tests hermetic: webhook validation must not depend on live DNS.
func TestMain(m *testing.M) {
	realLookup := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]string, error) {
		switch {
		case strings.HasSuffix(host, ".example.com"), host == "example.com",
			host == "discord.gg", host == "discord.com", host == "hooks.slack.com":
			return []string{"93.184.216.34"}, nil // public documentation address
		case host == "private.test":
			return []string{"10.0.0.5"}, nil
		}
		if net.ParseIP(host) != nil {
			return []string{host}, nil
		}
		return realLookup(ctx, host)
	}
	os.Exit(m.Run())
}
