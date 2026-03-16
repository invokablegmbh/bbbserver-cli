package postman

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

const DefaultDocumenterURL = "https://documenter.getpostman.com/view/7658590/T1DwdET1?version=latest"

var generatedCollectionJSON string

type Collection struct {
	Item []Item `json:"item"`
}

type collectionWrapper struct {
	Collection Collection `json:"collection"`
}

type Item struct {
	Name    string   `json:"name"`
	Item    []Item   `json:"item"`
	Request *Request `json:"request"`
}

type Request struct {
	Method      string   `json:"method"`
	URL         URL      `json:"url"`
	Body        *Body    `json:"body"`
	Description string   `json:"description"`
	Header      []Header `json:"header"`
}

type Header struct {
	Key string `json:"key"`
}

type Body struct {
	Mode     string      `json:"mode"`
	Raw      string      `json:"raw"`
	FormData []BodyParam `json:"formdata"`
}

type BodyParam struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	Description interface{} `json:"description"`
	Type        string      `json:"type"`
	Disabled    bool        `json:"disabled"`
}

type URL struct {
	Raw   string      `json:"raw"`
	Path  interface{} `json:"path"`
	Query []Query     `json:"query"`
}

func (u *URL) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*u = URL{}
		return nil
	}

	if strings.HasPrefix(trimmed, "\"") {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		u.Raw = raw
		u.Path = nil
		u.Query = nil
		return nil
	}

	type alias URL
	var parsed alias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*u = URL(parsed)
	return nil
}

type Query struct {
	Key string `json:"key"`
}

type Endpoint struct {
	Groups      []string
	Name        string
	Method      string
	Path        string
	Description string
	QueryParams []string
	PathParams  []string
	HasBody     bool
	BodyFields  []BodyField
}

type BodyField struct {
	Key         string
	SampleValue string
	Description string
	IsFile      bool
}

func (e Endpoint) HasFileFields() bool {
	for _, f := range e.BodyFields {
		if f.IsFile {
			return true
		}
	}
	return false
}

func ParseFile(path string) ([]Endpoint, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Postman collection: %w", err)
	}
	return ParseBytes(content)
}

func ParseBytes(content []byte) ([]Endpoint, error) {
	var collection Collection
	if err := json.Unmarshal(content, &collection); err != nil {
		return nil, fmt.Errorf("parse Postman collection: %w", err)
	}

	if len(collection.Item) == 0 {
		var wrapper collectionWrapper
		if err := json.Unmarshal(content, &wrapper); err == nil && len(wrapper.Collection.Item) > 0 {
			collection = wrapper.Collection
		}
	}

	endpoints := make([]Endpoint, 0)
	walkItems(collection.Item, nil, &endpoints)

	sort.Slice(endpoints, func(i, j int) bool {
		left := strings.Join(append(endpoints[i].Groups, endpoints[i].Name), "/")
		right := strings.Join(append(endpoints[j].Groups, endpoints[j].Name), "/")
		return left < right
	})

	return endpoints, nil
}

func ParseEmbedded() ([]Endpoint, error) {
	if strings.TrimSpace(generatedCollectionJSON) == "" {
		return nil, fmt.Errorf("embedded collection is not generated; run make collection-generate before build")
	}
	return ParseBytes([]byte(generatedCollectionJSON))
}

func ParseReader(reader io.Reader) ([]Endpoint, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read Postman collection stream: %w", err)
	}
	return ParseBytes(content)
}

func walkItems(items []Item, groups []string, endpoints *[]Endpoint) {
	for _, item := range items {
		if item.Request != nil {
			hasBody := supportsBody(item.Request.Method)
			endpoint := Endpoint{
				Groups:      append([]string{}, groups...),
				Name:        item.Name,
				Method:      strings.ToUpper(strings.TrimSpace(item.Request.Method)),
				Path:        normalizePath(item.Request.URL),
				Description: sanitizeDescription(item.Request.Description),
				QueryParams: extractQueryParams(item.Request.URL),
				HasBody:     hasBody,
			}
			if hasBody {
				endpoint.BodyFields = extractBodyFields(item.Request.Body)
			}
			endpoint.PathParams = extractPathParams(endpoint.Path)
			*endpoints = append(*endpoints, endpoint)
			continue
		}

		nextGroups := append(append([]string{}, groups...), item.Name)
		walkItems(item.Item, nextGroups, endpoints)
	}
}

func sanitizeDescription(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	withoutTags := regexp.MustCompile(`(?s)<[^>]*>`).ReplaceAllString(trimmed, "")
	cleaned := html.UnescapeString(withoutTags)
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	lines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func extractBodyFields(body *Body) []BodyField {
	if body == nil {
		return nil
	}

	if len(body.FormData) > 0 {
		fields := make([]BodyField, 0, len(body.FormData))
		for _, field := range body.FormData {
			key := strings.TrimSpace(field.Key)
			if key == "" || field.Disabled {
				continue
			}
			fields = append(fields, BodyField{
				Key:         key,
				SampleValue: sampleValueString(field.Value),
				Description: extractDescription(field.Description),
				IsFile:      strings.EqualFold(strings.TrimSpace(field.Type), "file"),
			})
		}
		return fields
	}

	return nil
}

func sampleValueString(value interface{}) string {
	if value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return typed
	case bool, float64, float32, int, int64, uint64:
		return fmt.Sprintf("%v", typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(data)
	}
}

func extractDescription(value interface{}) string {
	if value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return sanitizeDescription(typed)
	case map[string]interface{}:
		if content, ok := typed["content"]; ok {
			return sanitizeDescription(sampleValueString(content))
		}
		if text, ok := typed["text"]; ok {
			return sanitizeDescription(sampleValueString(text))
		}
		return sanitizeDescription(sampleValueString(typed))
	default:
		return sanitizeDescription(sampleValueString(typed))
	}
}

func supportsBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}


func extractQueryParams(urlValue URL) []string {
	query := urlValue.Query
	if len(query) == 0 {
		return extractQueryFromRaw(urlValue.Raw)
	}
	keys := make([]string, 0, len(query))
	for _, q := range query {
		key := strings.TrimSpace(q.Key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return extractQueryFromRaw(urlValue.Raw)
	}

	return uniqueStrings(keys)
}

func normalizePath(urlValue URL) string {
	if path, ok := toPathSegments(urlValue.Path); ok {
		clean := make([]string, 0, len(path))
		for _, segment := range path {
			trimmed := strings.Trim(segment, " /")
			if trimmed != "" {
				clean = append(clean, trimmed)
			}
		}
		if len(clean) == 0 {
			return "/"
		}
		return "/" + strings.Join(clean, "/")
	}

	raw := strings.TrimSpace(urlValue.Raw)
	if raw == "" {
		return "/"
	}

	if idx := strings.Index(raw, "{{api_base}}"); idx >= 0 {
		remaining := strings.TrimSpace(raw[idx+len("{{api_base}}"):])
		if remaining == "" {
			return "/"
		}
		if cut := strings.IndexAny(remaining, "?#"); cut >= 0 {
			remaining = remaining[:cut]
		}
		if !strings.HasPrefix(remaining, "/") {
			remaining = "/" + remaining
		}
		return remaining
	}

	if parsed, err := url.Parse(raw); err == nil {
		if parsed.Path != "" {
			return parsed.Path
		}
	}

	return "/"
}

func extractQueryFromRaw(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if idx := strings.Index(raw, "?"); idx >= 0 {
		queryString := raw[idx+1:]
		if cut := strings.Index(queryString, "#"); cut >= 0 {
			queryString = queryString[:cut]
		}
		queryValues, err := url.ParseQuery(queryString)
		if err == nil {
			keys := make([]string, 0, len(queryValues))
			for key := range queryValues {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			return keys
		}
	}

	return nil
}

func toPathSegments(path interface{}) ([]string, bool) {
	switch val := path.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, segment := range val {
			result = append(result, fmt.Sprintf("%v", segment))
		}
		return result, true
	case []string:
		return val, true
	case string:
		if strings.TrimSpace(val) == "" {
			return nil, false
		}
		parts := strings.Split(val, "/")
		return parts, true
	default:
		return nil, false
	}
}

func extractPathParams(path string) []string {
	result := make([]string, 0)

	for _, match := range regexp.MustCompile(`\{([^{}]+)\}`).FindAllStringSubmatch(path, -1) {
		if len(match) > 1 {
			result = append(result, strings.TrimSpace(match[1]))
		}
	}
	for _, match := range regexp.MustCompile(`:([A-Za-z0-9_\-]+)`).FindAllStringSubmatch(path, -1) {
		if len(match) > 1 {
			result = append(result, strings.TrimSpace(match[1]))
		}
	}
	for _, match := range regexp.MustCompile(`\{\{([^{}]+)\}\}`).FindAllStringSubmatch(path, -1) {
		if len(match) > 1 {
			result = append(result, strings.TrimSpace(match[1]))
		}
	}

	return uniqueStrings(result)
}

func Slug(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	lower = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(lower, "-")
	lower = strings.Trim(lower, "-")
	if lower == "" {
		return "endpoint"
	}
	return lower
}

func FlagName(value string) string {
	return Slug(value)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
