package api

import "testing"

func TestBuildURL_DeduplicatesBasePathPrefix(t *testing.T) {
	got, err := buildURL(
		"https://app.bbbserver.de/bbb-system-api",
		"/bbb-system-api/conference-rooms/list",
		nil,
	)
	if err != nil {
		t.Fatalf("buildURL() error = %v", err)
	}

	want := "https://app.bbbserver.de/bbb-system-api/conference-rooms/list"
	if got != want {
		t.Fatalf("buildURL() = %q, want %q", got, want)
	}
}

func TestBuildURL_AppendsPathWhenPrefixNotPresent(t *testing.T) {
	got, err := buildURL(
		"https://app.bbbserver.de/bbb-system-api",
		"/conference-rooms/list",
		nil,
	)
	if err != nil {
		t.Fatalf("buildURL() error = %v", err)
	}

	want := "https://app.bbbserver.de/bbb-system-api/conference-rooms/list"
	if got != want {
		t.Fatalf("buildURL() = %q, want %q", got, want)
	}
}

func TestBuildURL_RespectsQueryAndPathCleaning(t *testing.T) {
	got, err := buildURL(
		"https://example.com/api/",
		"api//v1/../v1/items",
		map[string]string{"page": "2", "empty": ""},
	)
	if err != nil {
		t.Fatalf("buildURL() error = %v", err)
	}

	want := "https://example.com/api/v1/items?page=2"
	if got != want {
		t.Fatalf("buildURL() = %q, want %q", got, want)
	}
}
