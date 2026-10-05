package internal

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/sdk/go/module/netguard"
)

// lookupHost resolves webhook hosts for the private-address check; tests stub it.
var lookupHost = net.DefaultResolver.LookupHost

// webhookOpts is the netguard configuration for user-supplied webhook targets.
var webhookOpts = netguard.Options{RequireHTTPS: true, Timeout: 10 * time.Second}

// appriseOpts is the configuration for the admin-configured Apprise server,
// which is commonly a LAN or loopback service.
var appriseOpts = netguard.Options{AllowPrivate: true, AllowLoopback: true, Timeout: 10 * time.Second}

// validateWebhookURL rejects non-https and private/loopback/link-local/metadata
// targets (RULE-VAL-2). The resolved addresses are re-checked at dial time by
// the netguard client.
func validateWebhookURL(ctx context.Context, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := netguard.ValidateURL(raw, netguard.UserURL, webhookOpts); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	host := u.Hostname()
	if parseAddr(host).IsValid() {
		return nil // literal IP already checked by ValidateURL
	}
	addrs, err := lookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("webhook host lookup failed: %w", err)
	}
	for _, a := range addrs {
		ip, perr := netip.ParseAddr(a)
		if perr != nil {
			continue
		}
		if err := netguard.CheckAddr(ip, netguard.UserURL, webhookOpts); err != nil {
			return fmt.Errorf("webhook host %q resolves to %q: %w", host, a, err)
		}
	}
	return nil
}

func parseAddr(h string) netip.Addr {
	a, _ := netip.ParseAddr(strings.Trim(h, "[]"))
	return a
}

// validateAppriseURL checks the admin-configured Apprise server endpoint:
// private/loopback allowed, metadata/link-local blocked.
func validateAppriseURL(raw string) error {
	if err := netguard.ValidateURL(raw, netguard.Integration, appriseOpts); err != nil {
		return fmt.Errorf("invalid apprise_url: %w", err)
	}
	return nil
}
