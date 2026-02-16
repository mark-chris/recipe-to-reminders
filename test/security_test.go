package test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestValidateURL_AcceptsHTTP(t *testing.T) {
	_, err := parser.ValidateURL("http://example.com/recipe")
	if err != nil {
		t.Fatalf("expected http to be accepted, got: %v", err)
	}
}

func TestValidateURL_AcceptsHTTPS(t *testing.T) {
	_, err := parser.ValidateURL("https://example.com/recipe")
	if err != nil {
		t.Fatalf("expected https to be accepted, got: %v", err)
	}
}

func TestValidateURL_RejectsFileScheme(t *testing.T) {
	_, err := parser.ValidateURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected file:// to be rejected")
	}
}

func TestValidateURL_RejectsFTPScheme(t *testing.T) {
	_, err := parser.ValidateURL("ftp://example.com/recipe")
	if err == nil {
		t.Fatal("expected ftp:// to be rejected")
	}
}

func TestValidateURL_RejectsNoScheme(t *testing.T) {
	_, err := parser.ValidateURL("example.com/recipe")
	if err == nil {
		t.Fatal("expected missing scheme to be rejected")
	}
}

func TestValidateURL_RejectsEmptyHost(t *testing.T) {
	_, err := parser.ValidateURL("http://")
	if err == nil {
		t.Fatal("expected empty host to be rejected")
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"0.0.0.0",
		"224.0.0.1",
		"::1",
		"fe80::1",
	}
	for _, ip := range blocked {
		t.Run(ip, func(t *testing.T) {
			if !parser.IsBlockedIP(net.ParseIP(ip)) {
				t.Errorf("expected %s to be blocked", ip)
			}
		})
	}

	allowed := []string{
		"93.184.216.34",
		"8.8.8.8",
		"2606:2800:220:1:248:1893:25c8:1946",
	}
	for _, ip := range allowed {
		t.Run(ip, func(t *testing.T) {
			if parser.IsBlockedIP(net.ParseIP(ip)) {
				t.Errorf("expected %s to be allowed", ip)
			}
		})
	}
}

func TestFetcher_RejectsOversizedResponse(t *testing.T) {
	// Server returns 10MB of data (well over 1024-byte limit)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		for i := 0; i < 10*1024; i++ {
			_, _ = fmt.Fprint(w, "x")
			for j := 0; j < 1024; j++ {
				_, _ = fmt.Fprint(w, "x")
			}
		}
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithMaxBodySize(1024), parser.WithAllowLoopback(true))
	_, err := f.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}

func TestFetcher_RespectsRedirectLimit(t *testing.T) {
	redirectCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount <= 5 {
			http.Redirect(w, r, fmt.Sprintf("/?attempt=%d", redirectCount), http.StatusFound)
			return
		}
		_, _ = fmt.Fprint(w, "final")
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithAllowLoopback(true))
	_, err := f.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected redirect limit error")
	}
}

func TestValidateURL_RejectsJavascriptScheme(t *testing.T) {
	_, err := parser.ValidateURL("javascript:alert(1)")
	if err == nil {
		t.Fatal("expected javascript: to be rejected")
	}
}

func TestValidateURL_RejectsDataScheme(t *testing.T) {
	_, err := parser.ValidateURL("data:text/html,<h1>hello</h1>")
	if err == nil {
		t.Fatal("expected data: to be rejected")
	}
}

func TestValidateURL_RejectsGopherScheme(t *testing.T) {
	_, err := parser.ValidateURL("gopher://evil.com/")
	if err == nil {
		t.Fatal("expected gopher: to be rejected")
	}
}

func TestValidateURL_PreservesPath(t *testing.T) {
	u, err := parser.ValidateURL("https://example.com/recipes/beef-stew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Path != "/recipes/beef-stew" {
		t.Errorf("expected path /recipes/beef-stew, got %s", u.Path)
	}
}

func TestIsBlockedIP_NilIP(t *testing.T) {
	if !parser.IsBlockedIP(nil) {
		t.Error("expected nil IP to be blocked")
	}
}

func TestFetcher_BlocksLoopbackByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "should not reach here")
	}))
	defer ts.Close()

	// Default fetcher (no allowLoopback) should reject 127.0.0.1
	f := parser.NewFetcher()
	_, err := f.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected loopback to be blocked by default")
	}
}

func TestFetcher_BlocksRedirectToBlockedIP(t *testing.T) {
	// Simulate a redirect from a public server to a blocked internal IP.
	// The DialContext hook should reject the connection to the redirect target.
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "internal secret")
	}))
	defer blocked.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blocked.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Default fetcher (no allowLoopback) should block the redirect target
	f := parser.NewFetcher()
	_, err := f.Fetch(context.Background(), redirector.URL)
	if err == nil {
		t.Fatal("expected redirect to loopback to be blocked")
	}
}

func TestFetcher_RejectsNon200Status(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithAllowLoopback(true))
	_, err := f.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetcher_SuccessfulFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body>Hello</body></html>")
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithAllowLoopback(true))
	body, err := f.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if string(body) != "<html><body>Hello</body></html>" {
		t.Errorf("unexpected body: %s", body)
	}
}
