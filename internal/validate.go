package internal

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateWebhookURL(ctx context.Context, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL must include a host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL must not target private or link-local IP %q", host)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("webhook host lookup failed: %w", err)
	}
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("webhook host %q resolves to private IP %q", host, a)
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
