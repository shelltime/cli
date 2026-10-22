package model

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http/httpproxy"
)

// supportedProxySchemes lists the proxy schemes natively handled by net/http.
// net/http treats socks5 the same as socks5h: hostnames are resolved by the proxy.
var supportedProxySchemes = map[string]bool{
	"http":    true,
	"https":   true,
	"socks5":  true,
	"socks5h": true,
}

// sharedTransport is the base transport used by every outbound HTTP client.
// It defaults to a clone of http.DefaultTransport, which honors the
// HTTP_PROXY / HTTPS_PROXY / NO_PROXY environment variables.
var sharedTransport atomic.Pointer[http.Transport]

func init() {
	sharedTransport.Store(http.DefaultTransport.(*http.Transport).Clone())
}

// ConfigureProxy routes all outbound HTTP traffic through the configured proxy.
// A nil config or an empty URL keeps the environment-based proxy behavior.
func ConfigureProxy(cfg *ProxyConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.URL) == "" {
		return nil
	}

	proxyURL, err := ParseProxyURL(cfg.URL)
	if err != nil {
		return err
	}

	proxyFunc := (&httpproxy.Config{
		HTTPProxy:  proxyURL.String(),
		HTTPSProxy: proxyURL.String(),
		NoProxy:    strings.Join(cfg.NoProxy, ","),
	}).ProxyFunc()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		return proxyFunc(r.URL)
	}
	sharedTransport.Store(transport)
	return nil
}

// ParseProxyURL validates a proxy URL. A bare "host:port" is treated as http.
func ParseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if !supportedProxySchemes[scheme] {
		return nil, fmt.Errorf("unsupported proxy scheme %q: use http, https, socks5 or socks5h", u.Scheme)
	}
	u.Scheme = scheme

	if u.Hostname() == "" {
		return nil, fmt.Errorf("invalid proxy url %q: missing host", RedactProxyURL(raw))
	}
	return u, nil
}

// RedactProxyURL hides the password of a proxy URL for display and logging.
// Like ParseProxyURL it reads a URL without a scheme as http, so a bare
// "user:pass@host:port" is redacted too. When the URL doesn't parse into user
// info (a password with an unescaped '/', '%', '?' or '#'), everything between
// the scheme and the last '@' is hidden.
func RedactProxyURL(raw string) string {
	bare := !strings.Contains(raw, "://")
	toParse := raw
	if bare {
		toParse = "http://" + raw
	}
	if u, err := url.Parse(toParse); err == nil && u.User != nil {
		if bare {
			return strings.TrimPrefix(u.Redacted(), "http://")
		}
		return u.Redacted()
	}

	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return raw
	}
	start := 0
	if i := strings.Index(raw, "://"); i >= 0 && i < at {
		start = i + len("://")
	}
	return raw[:start] + "xxxxx" + raw[at:]
}

// HTTPTransport returns the shared, proxy-aware base transport.
func HTTPTransport() http.RoundTripper {
	return sharedTransport.Load()
}

// NewHTTPClient creates an OTEL-instrumented HTTP client on the shared transport.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(HTTPTransport()),
	}
}
