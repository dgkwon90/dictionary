package http

import (
	"crypto/subtle"
	"net"
	nethttp "net/http"
	"strings"
)

// Secure wraps the router with the local API's trust boundary (review R-01):
// every request must present the correct bearer token and target a loopback
// Host header. Both checks run before the request reaches routing, so an
// unauthenticated request or one whose Host was spoofed via DNS rebinding never
// reaches a handler. /healthz is exempt — it is side-effect-free and safe to
// leave reachable for a basic liveness check.
//
// The Host check matters independently of the token: DNS rebinding lets an
// attacker's domain resolve to 127.0.0.1, so the browser connects to the real
// loopback server, but the Host header it sends is still the attacker's domain
// — checking bind address alone (config.validateLoopbackAddr) does not catch
// this, only inspecting the header on each request does.
func Secure(next nethttp.Handler, token string) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if !isLoopbackHost(r.Host) {
			nethttp.Error(w, "forbidden host", nethttp.StatusForbidden)
			return
		}
		if !validToken(r, token) {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validToken(r *nethttp.Request, token string) bool {
	if token == "" {
		return false // misconfiguration: never treat "no token configured" as "open"
	}
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	provided := strings.TrimPrefix(auth, prefix)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func isLoopbackHost(host string) bool {
	h := host
	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
