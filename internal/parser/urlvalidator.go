package parser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultMaxBodySize = 5 * 1024 * 1024 // 5MB
	maxRedirects       = 3
	connectTimeout     = 5 * time.Second
	tlsTimeout         = 5 * time.Second
	headerTimeout      = 10 * time.Second
	requestTimeout     = 15 * time.Second
)

var blockedNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"0.0.0.0/8",
		"::1/128",
		"fe80::/10",
		"ff00::/8",
		"::/128",
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %s: %v", cidr, err))
		}
		blockedNetworks = append(blockedNetworks, network)
	}
}

// ValidateURL checks that the URL has an allowed scheme and a non-empty host.
func ValidateURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL must include a host")
	}
	return u, nil
}

// IsBlockedIP returns true if the IP falls within any blocked CIDR range.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	for _, network := range blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Fetcher handles safe HTTP fetching with SSRF protections.
type Fetcher struct {
	maxBodySize   int64
	allowLoopback bool
}

// FetcherOption configures a Fetcher.
type FetcherOption func(*Fetcher)

// WithMaxBodySize overrides the default response body size limit.
func WithMaxBodySize(n int64) FetcherOption {
	return func(f *Fetcher) { f.maxBodySize = n }
}

// WithAllowLoopback permits fetching from loopback addresses (for testing only).
func WithAllowLoopback(allow bool) FetcherOption {
	return func(f *Fetcher) { f.allowLoopback = allow }
}

// NewFetcher creates a Fetcher with the given options.
func NewFetcher(opts ...FetcherOption) *Fetcher {
	f := &Fetcher{
		maxBodySize: defaultMaxBodySize,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Fetch validates a URL, resolves DNS, checks IPs, follows redirects safely,
// and returns the response body (capped at maxBodySize).
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	transport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch recipe from URL")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(dialCtx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch recipe from URL")
			}
			for _, ip := range ips {
				if !f.allowLoopback && IsBlockedIP(ip.IP) {
					return nil, fmt.Errorf("failed to fetch recipe from URL")
				}
			}
			dialer := &net.Dialer{Timeout: connectTimeout}
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:  tlsTimeout,
		ResponseHeaderTimeout: headerTimeout,
	}

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// Re-validate redirect target scheme and host (IP check happens in DialContext)
			_, err := ValidateURL(req.URL.String())
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	req.Header.Set("User-Agent", "RecipeToReminders/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req) // #nosec G704 -- URL is validated, DNS resolved, IPs checked against blocklist
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}

	if resp.ContentLength > f.maxBodySize {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	if int64(len(body)) > f.maxBodySize {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	return body, nil
}
