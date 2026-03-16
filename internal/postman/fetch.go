package postman

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const documenterAPIBase = "https://documenter.gw.postman.com/api/collections"

func ParseFromURL(collectionURL string) ([]Endpoint, error) {
	body, err := FetchCollection(collectionURL)
	if err != nil {
		return nil, err
	}
	return ParseBytes(body)
}

func FetchCollection(collectionURL string) ([]byte, error) {
	resolved, err := resolveCollectionAPIURL(collectionURL)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, resolved, nil)
	if err != nil {
		return nil, fmt.Errorf("build collection request: %w", err)
	}
	req.Header.Set("User-Agent", "bbbserver-cli/collection-fetch")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download collection: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read collection response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download collection failed with status %d", resp.StatusCode)
	}

	if json.Valid(body) {
		return body, nil
	}

	return nil, fmt.Errorf("downloaded collection is not valid JSON")
}

func resolveCollectionAPIURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = DefaultDocumenterURL
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid collection URL: %w", err)
	}

	if parsed.Host == "documenter.gw.postman.com" && strings.HasPrefix(parsed.Path, "/api/collections/") {
		return parsed.String(), nil
	}

	if parsed.Host != "documenter.getpostman.com" {
		return "", fmt.Errorf("unsupported collection URL host: %s", parsed.Host)
	}

	ownerID, publishedID, err := parseDocumenterPath(parsed.Path)
	if err != nil {
		return "", err
	}

	versionTag := strings.TrimSpace(parsed.Query().Get("version"))
	if versionTag == "" {
		versionTag = "latest"
	}

	builder := fmt.Sprintf("%s/%s/%s", documenterAPIBase, ownerID, publishedID)
	query := url.Values{}
	query.Set("segregateAuth", "true")
	query.Set("versionTag", versionTag)

	return builder + "?" + query.Encode(), nil
}

func parseDocumenterPath(path string) (ownerID string, publishedID string, err error) {
	match := regexp.MustCompile(`^/view/([^/]+)/([^/]+)$`).FindStringSubmatch(strings.TrimSpace(path))
	if len(match) != 3 {
		return "", "", fmt.Errorf("unsupported documenter URL path: %s", path)
	}
	return match[1], match[2], nil
}
