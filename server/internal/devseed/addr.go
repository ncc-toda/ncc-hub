package devseed

import (
	"fmt"
	"net"
	"strings"
)

// RequireLoopback は --dev の待ち受けがループバックであることを要求する。
// 空のホスト（全インタフェース）や 0.0.0.0 は拒否する。
func RequireLoopback(addr string) error {
	if !IsLoopbackAddr(addr) {
		return fmt.Errorf("--dev はループバック専用です: %s", addr)
	}
	return nil
}

// IsLoopbackAddr は host:port がループバックかどうかを返す。
func IsLoopbackAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
