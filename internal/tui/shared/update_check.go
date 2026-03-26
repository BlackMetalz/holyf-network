package shared

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const GithubLatestReleaseAPI = "https://api.github.com/repos/BlackMetalz/holyf-network/releases/latest"

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
}

func CheckForUpdate(ctx context.Context, client *http.Client, currentVersion string) (string, bool) {
	return CheckForUpdateWithURL(ctx, client, GithubLatestReleaseAPI, currentVersion)
}

func CheckForUpdateWithURL(ctx context.Context, client *http.Client, apiURL string, currentVersion string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(apiURL), nil)
	if err != nil {
		return "", false
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var release githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", false
	}

	latest := strings.TrimSpace(release.TagName)
	current := strings.TrimSpace(currentVersion)
	if latest == "" || latest == current {
		return "", false
	}

	return latest, true
}
