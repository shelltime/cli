package model

import (
	"context"
	"runtime"
	"time"
)

// FetchLatestCLIReleaseQuery asks the server for the latest release plus the
// archive for this platform. downloadURL/checksumsURL come back pointing at the
// server's proxy, so a client that cannot reach github.com can still update.
const FetchLatestCLIReleaseQuery = `query fetchLatestCLIRelease($goos: String!, $goarch: String!) {
	metaData {
		cliVersion
		latestCLIRelease {
			tag
			publishedAt
			checksumsURL
			asset(goos: $goos, goarch: $goarch) {
				name
				size
				sha256
				downloadURL
			}
		}
	}
}`

// CLIReleaseAssetInfo is one platform's release archive.
type CLIReleaseAssetInfo struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	DownloadURL string `json:"downloadURL"`
}

// LatestCLIRelease is the newest published CLI release.
type LatestCLIRelease struct {
	Tag          string               `json:"tag"`
	PublishedAt  time.Time            `json:"publishedAt"`
	ChecksumsURL string               `json:"checksumsURL"`
	Asset        *CLIReleaseAssetInfo `json:"asset"`
}

type latestCLIReleaseData struct {
	MetaData struct {
		CliVersion       string           `json:"cliVersion"`
		LatestCLIRelease LatestCLIRelease `json:"latestCLIRelease"`
	} `json:"metaData"`
}

// FetchLatestCLIRelease queries the server for the latest release for the
// running platform.
func FetchLatestCLIRelease(ctx context.Context, config ShellTimeConfig) (LatestCLIRelease, error) {
	ctx, span := modelTracer.Start(ctx, "cliRelease.fetchLatest")
	defer span.End()

	goos, goarch := CurrentPlatform()

	var result GraphQLResponse[latestCLIReleaseData]
	err := SendGraphQLRequest(GraphQLRequestOptions[GraphQLResponse[latestCLIReleaseData]]{
		Context: ctx,
		Endpoint: Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		Query: FetchLatestCLIReleaseQuery,
		Variables: map[string]interface{}{
			"goos":   goos,
			"goarch": goarch,
		},
		Response: &result,
		Timeout:  10 * time.Second,
	})
	if err != nil {
		return LatestCLIRelease{}, err
	}

	return result.Data.MetaData.LatestCLIRelease, nil
}

// CurrentPlatformArchiveName returns the release archive filename for the
// running platform.
func CurrentPlatformArchiveName() (string, error) {
	return BuildArchiveName(runtime.GOOS, runtime.GOARCH)
}
