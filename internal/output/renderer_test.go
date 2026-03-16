package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestHumanOutputNormalizesTypedMaps(t *testing.T) {
	stdout := &bytes.Buffer{}
	r := Renderer{Mode: "human", Stdout: stdout}

	value := map[string]string{
		"base_url": "https://api.example.com",
		"api_key":  "****1234",
	}

	if err := r.Data(value); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "map[") {
		t.Fatalf("human output contains raw map dump: %q", out)
	}
	if !strings.Contains(out, "base_url:") {
		t.Fatalf("human output missing base_url key: %q", out)
	}
	if !strings.Contains(out, "api_key:") {
		t.Fatalf("human output missing api_key key: %q", out)
	}
}

func TestHumanOutputPrintsNestedValuesMultiline(t *testing.T) {
	stdout := &bytes.Buffer{}
	r := Renderer{Mode: "human", Stdout: stdout}

	value := map[string]any{
		"status": "ok",
		"meta": map[string]any{
			"page":  1,
			"count": 2,
		},
	}

	if err := r.Data(value); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "map[") {
		t.Fatalf("human output contains raw map dump: %q", out)
	}
	if !strings.Contains(out, "meta:") {
		t.Fatalf("human output missing nested key header: %q", out)
	}
	if !strings.Contains(out, "  count: 2") {
		t.Fatalf("human output missing nested count field: %q", out)
	}
	if !strings.Contains(out, "  page: 1") {
		t.Fatalf("human output missing nested page field: %q", out)
	}
}

func TestHumanOutputPrioritizesIDAndNameInObject(t *testing.T) {
	stdout := &bytes.Buffer{}
	r := Renderer{Mode: "human", Stdout: stdout}

	value := map[string]any{
		"status": "active",
		"name":   "Room A",
		"id":     42,
	}

	if err := r.Data(value); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	out := stdout.String()
	idIndex := strings.Index(out, "id:")
	nameIndex := strings.Index(out, "name:")
	statusIndex := strings.Index(out, "status:")
	if idIndex == -1 || nameIndex == -1 || statusIndex == -1 {
		t.Fatalf("missing expected keys in output: %q", out)
	}
	if !(idIndex < nameIndex && nameIndex < statusIndex) {
		t.Fatalf("expected id then name then status order, got: %q", out)
	}
}

func TestHumanOutputPrioritizesIDAndNameInTable(t *testing.T) {
	stdout := &bytes.Buffer{}
	r := Renderer{Mode: "human", Stdout: stdout}

	value := []any{
		map[string]any{"status": "active", "name": "Room A", "id": 1},
		map[string]any{"status": "inactive", "name": "Room B", "id": 2},
	}

	if err := r.Data(value); err != nil {
		t.Fatalf("Data() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected table header, got empty output")
	}
	header := strings.Fields(lines[0])
	if len(header) < 3 {
		t.Fatalf("expected at least 3 columns in header, got %v", header)
	}
	if header[0] != "id" || header[1] != "name" {
		t.Fatalf("expected header to start with id name, got %v", header)
	}
}
