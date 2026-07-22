package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
	})
}

func TestSecureHealthzExemptFromAuthAndHost(t *testing.T) {
	handler := Secure(okHandler(), "correct-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)
	request.Host = "evil.example.com"

	handler.ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSecureRejectsMissingToken(t *testing.T) {
	handler := Secure(okHandler(), "correct-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox", nil)
	request.Host = "127.0.0.1:48989"

	handler.ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusUnauthorized)
	}
}

func TestSecureRejectsWrongToken(t *testing.T) {
	handler := Secure(okHandler(), "correct-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox", nil)
	request.Host = "127.0.0.1:48989"
	request.Header.Set("Authorization", "Bearer wrong-token")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusUnauthorized)
	}
}

func TestSecureRejectsEmptyConfiguredToken(t *testing.T) {
	handler := Secure(okHandler(), "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox", nil)
	request.Host = "127.0.0.1:48989"
	request.Header.Set("Authorization", "Bearer ")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusUnauthorized {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusUnauthorized)
	}
}

func TestSecureRejectsNonLoopbackHost(t *testing.T) {
	tests := []string{
		"evil.example.com",
		"evil.example.com:48989",
		"192.168.1.5:48989",
		"attacker-controlled-domain-that-rebinds-to-loopback.com",
	}
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			handler := Secure(okHandler(), "correct-token")
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox", nil)
			request.Host = host
			request.Header.Set("Authorization", "Bearer correct-token")

			handler.ServeHTTP(recorder, request)

			if recorder.Code != nethttp.StatusForbidden {
				t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusForbidden)
			}
		})
	}
}

func TestSecureAllowsLoopbackHostWithValidToken(t *testing.T) {
	tests := []string{
		"127.0.0.1:48989",
		"localhost:48989",
		"[::1]:48989",
	}
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			handler := Secure(okHandler(), "correct-token")
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox", nil)
			request.Host = host
			request.Header.Set("Authorization", "Bearer correct-token")

			handler.ServeHTTP(recorder, request)

			if recorder.Code != nethttp.StatusOK {
				t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
			}
		})
	}
}

// TestSecureHostCheckDefeatsDNSRebinding simulates the R-01 DNS-rebinding
// scenario: the attacker's domain resolves to 127.0.0.1 (so a naive bind-address
// check would see nothing wrong), but the browser still sends the attacker's
// hostname as the Host header, which Secure must reject even with a valid token.
func TestSecureHostCheckDefeatsDNSRebinding(t *testing.T) {
	handler := Secure(okHandler(), "correct-token")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures", nil)
	request.Host = "rebound.attacker.example:48989" // resolves to 127.0.0.1 at DNS level
	request.Header.Set("Authorization", "Bearer correct-token")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusForbidden)
	}
}
