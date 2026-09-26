package model

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetSharedTransport restores the default env-based transport after a test.
func resetSharedTransport(t *testing.T) {
	t.Cleanup(func() {
		sharedTransport.Store(http.DefaultTransport.(*http.Transport).Clone())
	})
}

func proxyFor(t *testing.T, target string) *url.URL {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	u, err := sharedTransport.Load().Proxy(req)
	require.NoError(t, err)
	return u
}

func TestParseProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "http", raw: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "https", raw: "https://proxy.corp:443", want: "https://proxy.corp:443"},
		{name: "socks5", raw: "socks5://127.0.0.1:1080", want: "socks5://127.0.0.1:1080"},
		{name: "socks5h with auth", raw: "socks5h://u:p@127.0.0.1:1080", want: "socks5h://u:p@127.0.0.1:1080"},
		{name: "uppercase scheme", raw: "SOCKS5://127.0.0.1:1080", want: "socks5://127.0.0.1:1080"},
		{name: "bare host port", raw: "127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{name: "surrounding spaces", raw: "  http://p:1  ", want: "http://p:1"},
		{name: "socks4 rejected", raw: "socks4://127.0.0.1:1080", wantErr: "unsupported proxy scheme"},
		{name: "ftp rejected", raw: "ftp://127.0.0.1:21", wantErr: "unsupported proxy scheme"},
		{name: "missing host", raw: "http://:8080", wantErr: "missing host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParseProxyURL(tt.raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, u.String())
		})
	}
}

func TestRedactProxyURL(t *testing.T) {
	assert.Equal(t, "socks5://u:xxxxx@h:1", RedactProxyURL("socks5://u:secret@h:1"))
	assert.Equal(t, "http://h:1", RedactProxyURL("http://h:1"))
	assert.Equal(t, "", RedactProxyURL(""))
}

func TestConfigureProxy_NilOrEmptyKeepsEnvBehavior(t *testing.T) {
	resetSharedTransport(t)
	before := sharedTransport.Load()

	require.NoError(t, ConfigureProxy(nil))
	require.NoError(t, ConfigureProxy(&ProxyConfig{URL: "  "}))
	assert.Same(t, before, sharedTransport.Load())
	assert.NotNil(t, before.Proxy, "default transport should use the environment proxy func")
}

func TestConfigureProxy_InvalidKeepsTransport(t *testing.T) {
	resetSharedTransport(t)
	before := sharedTransport.Load()

	err := ConfigureProxy(&ProxyConfig{URL: "socks4://127.0.0.1:1080"})
	require.Error(t, err)
	assert.Same(t, before, sharedTransport.Load())
}

func TestConfigureProxy_ProxySelection(t *testing.T) {
	resetSharedTransport(t)
	require.NoError(t, ConfigureProxy(&ProxyConfig{
		URL:     "socks5h://127.0.0.1:1080",
		NoProxy: []string{".corp.example.com", "10.0.0.0/8"},
	}))

	u := proxyFor(t, "https://api.shelltime.xyz/api/v1/track")
	require.NotNil(t, u)
	assert.Equal(t, "socks5h://127.0.0.1:1080", u.String())

	u = proxyFor(t, "http://github.com/")
	require.NotNil(t, u, "plain http requests are proxied too")

	assert.Nil(t, proxyFor(t, "https://git.corp.example.com/"), "noProxy domain suffix bypasses proxy")
	assert.Nil(t, proxyFor(t, "http://10.1.2.3/"), "noProxy CIDR bypasses proxy")
	assert.Nil(t, proxyFor(t, "http://127.0.0.1:9999/"), "loopback always bypasses proxy")
	assert.Nil(t, proxyFor(t, "http://localhost:9999/"), "localhost always bypasses proxy")
}

func TestNewHTTPClient_ThroughHTTPProxy(t *testing.T) {
	resetSharedTransport(t)

	var gotRequestURI, gotHost string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		gotHost = r.Host
		_, _ = w.Write([]byte("via-proxy"))
	}))
	defer proxy.Close()

	require.NoError(t, ConfigureProxy(&ProxyConfig{URL: proxy.URL}))

	resp, err := NewHTTPClient(5 * time.Second).Get("http://shelltime.test/api/v1/ping")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "via-proxy", string(body))
	assert.Equal(t, "http://shelltime.test/api/v1/ping", gotRequestURI, "proxy should receive an absolute-URI request")
	assert.Equal(t, "shelltime.test", gotHost)
}

func TestHTTPTransport_ThroughSOCKS5Proxy(t *testing.T) {
	resetSharedTransport(t)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello from " + r.Host))
	}))
	defer backend.Close()

	socksAddr, requestedHost := startSOCKS5Server(t, backend.Listener.Addr().String())
	require.NoError(t, ConfigureProxy(&ProxyConfig{URL: "socks5h://" + socksAddr}))

	client := &http.Client{Timeout: 5 * time.Second, Transport: HTTPTransport()}
	resp, err := client.Get("http://backend.test/")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "hello from backend.test", string(body))
	select {
	case host := <-requestedHost:
		assert.Equal(t, "backend.test:80", host, "hostname should be resolved by the proxy")
	case <-time.After(time.Second):
		t.Fatal("socks5 proxy was not used")
	}
}

// startSOCKS5Server runs a minimal no-auth SOCKS5 server that forwards every
// CONNECT to target and reports the requested destination.
func startSOCKS5Server(t *testing.T, target string) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	requested := make(chan string, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleSOCKS5Conn(conn, target, requested)
		}
	}()
	return ln.Addr().String(), requested
}

func handleSOCKS5Conn(conn net.Conn, target string, requested chan<- string) {
	defer conn.Close()

	// Greeting: VER, NMETHODS, METHODS...
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if _, err := io.ReadFull(conn, make([]byte, header[1])); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Request: VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return
		}
		name := make([]byte, l[0])
		if _, err := io.ReadFull(conn, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	select {
	case requested <- net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBuf)))):
	default:
	}

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	go func() { _, _ = io.Copy(upstream, conn) }()
	_, _ = io.Copy(conn, upstream)
}
