package model

import (
	"net/url"
	"strings"
)

// CLIReleaseProxyPath mirrors the server's release proxy route. Changing it is a
// breaking change for already-installed clients, which pin this path.
const CLIReleaseProxyPath = "/api/v1/cli/releases"

// ReleaseSource resolves release-asset URLs.
//
// The zero value means "fetch from GitHub directly", which preserves the
// historical behaviour for every existing caller. A non-empty ProxyBase routes
// downloads through the shelltime API instead, which is what makes updating work
// in regions where github.com is unreachable.
type ReleaseSource struct {
	ProxyBase string
}

// NewReleaseSource builds a source from a configured API endpoint. A blank or
// non-HTTP endpoint yields the direct-from-GitHub zero value.
func NewReleaseSource(apiEndpoint string) ReleaseSource {
	e := strings.TrimSpace(apiEndpoint)
	if !strings.HasPrefix(e, "http://") && !strings.HasPrefix(e, "https://") {
		return ReleaseSource{}
	}
	return ReleaseSource{ProxyBase: strings.TrimSuffix(e, "/")}
}

// IsProxy reports whether downloads go through the shelltime API.
func (s ReleaseSource) IsProxy() bool { return s.ProxyBase != "" }

// Direct returns the GitHub-direct source, used as a fallback when the proxy
// fails.
func (s ReleaseSource) Direct() ReleaseSource { return ReleaseSource{} }

// DownloadURL returns the URL for a release archive.
func (s ReleaseSource) DownloadURL(tag, assetName string) string {
	if !s.IsProxy() {
		return BuildDownloadURL(tag, assetName)
	}
	return s.ProxyBase + CLIReleaseProxyPath + "/" + url.PathEscape(tag) + "/" + url.PathEscape(assetName)
}

// ChecksumsURL returns the URL for the release's checksums.txt.
func (s ReleaseSource) ChecksumsURL(tag string) string {
	if !s.IsProxy() {
		return BuildChecksumsURL(tag)
	}
	return s.ProxyBase + CLIReleaseProxyPath + "/" + url.PathEscape(tag) + "/checksums.txt"
}
