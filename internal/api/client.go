package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/textproto"
	"net/http"
	"net/url"
	"os"
	stdpath "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	AuthModeAPIKey = "apikey"
	AuthModeBearer = "bearer"
)

type Client struct {
	BaseURL   string
	APIKey    string
	AuthMode  string
	HTTP      *http.Client
	UserAgent string
	Debug     bool
}

type Request struct {
	Method      string
	Path        string
	Query       map[string]string
	Body        []byte
	ContentType string
	RequireAuth bool
}

type Response struct {
	Status  int
	Headers http.Header
	Body    any
	RawBody []byte
}

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, &ValidationError{Message: "base URL is required"}
	}
	if req.RequireAuth && strings.TrimSpace(c.APIKey) == "" {
		return nil, &ValidationError{Message: "api key is required"}
	}

	if req.Method == "" {
		req.Method = http.MethodGet
	}

	finalURL, err := buildURL(c.BaseURL, req.Path, req.Query)
	if err != nil {
		return nil, &ValidationError{Message: err.Error()}
	}

	bodyReader := bytes.NewReader(req.Body)
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, finalURL, bodyReader)
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("build request: %v", err)}
	}

	httpReq.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		httpReq.Header.Set("User-Agent", c.UserAgent)
	}
	if len(req.Body) > 0 {
		contentType := req.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		httpReq.Header.Set("Content-Type", contentType)
	}

	if req.RequireAuth {
		switch strings.ToLower(strings.TrimSpace(c.AuthMode)) {
		case "", AuthModeAPIKey:
			httpReq.Header.Set("X-API-Key", c.APIKey)
		case AuthModeBearer:
			httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		default:
			return nil, &ValidationError{Message: "auth mode must be apikey or bearer"}
		}
	}

	if c.Debug {
		debugRequest(httpReq, req.Body)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapNetworkError(err)
	}
	defer httpResp.Body.Close()

	rawBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &NetworkError{Message: "read response body failed", Err: err}
	}

	if c.Debug {
		debugResponse(httpResp, rawBody)
	}

	parsedBody := parseResponseBody(httpResp.Header.Get("Content-Type"), rawBody)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, toAPIError(httpResp.StatusCode, httpResp.Header, parsedBody, rawBody)
	}

	return &Response{
		Status:  httpResp.StatusCode,
		Headers: httpResp.Header,
		Body:    parsedBody,
		RawBody: rawBody,
	}, nil
}

type MultipartField struct {
	Key      string
	Value    string
	FilePath string // non-empty means file upload
}

func (c *Client) DoMultipart(ctx context.Context, method, path string, query map[string]string, fields []MultipartField) (*Response, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, &ValidationError{Message: "base URL is required"}
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, &ValidationError{Message: "api key is required"}
	}

	finalURL, err := buildURL(c.BaseURL, path, query)
	if err != nil {
		return nil, &ValidationError{Message: err.Error()}
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, field := range fields {
		if field.FilePath != "" {
			f, err := os.Open(field.FilePath)
			if err != nil {
				return nil, &ValidationError{Message: fmt.Sprintf("open file %s: %v", field.FilePath, err)}
			}
			mimeType := detectMIMEType(field.FilePath)
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field.Key, filepath.Base(field.FilePath)))
			h.Set("Content-Type", mimeType)
			part, err := writer.CreatePart(h)
			if err != nil {
				f.Close()
				return nil, &ValidationError{Message: fmt.Sprintf("create form file: %v", err)}
			}
			if _, err := io.Copy(part, f); err != nil {
				f.Close()
				return nil, &ValidationError{Message: fmt.Sprintf("copy file data: %v", err)}
			}
			f.Close()
		} else {
			if err := writer.WriteField(field.Key, field.Value); err != nil {
				return nil, &ValidationError{Message: fmt.Sprintf("write field %s: %v", field.Key, err)}
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("close multipart writer: %v", err)}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, finalURL, &buf)
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("build request: %v", err)}
	}

	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.UserAgent != "" {
		httpReq.Header.Set("User-Agent", c.UserAgent)
	}

	switch strings.ToLower(strings.TrimSpace(c.AuthMode)) {
	case "", AuthModeAPIKey:
		httpReq.Header.Set("X-API-Key", c.APIKey)
	case AuthModeBearer:
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	default:
		return nil, &ValidationError{Message: "auth mode must be apikey or bearer"}
	}

	if c.Debug {
		debugRequest(httpReq, []byte(fmt.Sprintf("[multipart/form-data, %d bytes]", buf.Len())))
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapNetworkError(err)
	}
	defer httpResp.Body.Close()

	rawBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &NetworkError{Message: "read response body failed", Err: err}
	}

	if c.Debug {
		debugResponse(httpResp, rawBody)
	}

	parsedBody := parseResponseBody(httpResp.Header.Get("Content-Type"), rawBody)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, toAPIError(httpResp.StatusCode, httpResp.Header, parsedBody, rawBody)
	}

	return &Response{
		Status:  httpResp.StatusCode,
		Headers: httpResp.Header,
		Body:    parsedBody,
		RawBody: rawBody,
	}, nil
}

func buildURL(baseURL, requestPath string, query map[string]string) (string, error) {
	parsedBase, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	basePath := normalizeURLPath(parsedBase.Path)
	targetPath := normalizeURLPath(requestPath)
	parsedBase.Path = joinURLPaths(basePath, targetPath)

	values := parsedBase.Query()
	for key, value := range query {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values.Set(key, value)
	}
	parsedBase.RawQuery = values.Encode()

	return parsedBase.String(), nil
}

func normalizeURLPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	cleaned := stdpath.Clean(trimmed)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func joinURLPaths(basePath, targetPath string) string {
	if targetPath == "/" {
		return basePath
	}
	if basePath == "/" {
		return targetPath
	}

	if targetPath == basePath || strings.HasPrefix(targetPath, basePath+"/") {
		return targetPath
	}

	return strings.TrimRight(basePath, "/") + targetPath
}

func wrapNetworkError(err error) error {
	if err == nil {
		return nil
	}

	networkErr := &NetworkError{Message: "request failed", Err: err}
	if errorsIsTimeout(err) {
		networkErr.Timeout = true
		networkErr.Message = "request timed out"
	}
	return networkErr
}

func errorsIsTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	var netErr net.Error
	if ok := errors.As(err, &netErr); ok {
		return netErr.Timeout()
	}
	var timeoutErr timeout
	if ok := errors.As(err, &timeoutErr); ok {
		return timeoutErr.Timeout()
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func parseResponseBody(contentType string, raw []byte) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return map[string]any{}
	}

	if strings.Contains(strings.ToLower(contentType), "application/json") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			return parsed
		}
	}

	return map[string]any{"raw": trimmed}
}

func toAPIError(status int, headers http.Header, parsedBody any, rawBody []byte) error {
	requestID := headers.Get("X-Request-Id")
	if requestID == "" {
		requestID = headers.Get("X-Request-ID")
	}

	message := extractErrorMessage(parsedBody)
	if message == "" {
		message = strings.TrimSpace(string(rawBody))
	}
	if message == "" {
		message = http.StatusText(status)
	}

	errType := TypeAPI
	switch {
	case status == 401 || status == 403:
		errType = TypeAuth
	case status == 404:
		errType = TypeNotFound
	case status >= 500:
		errType = TypeServer
	}

	return &APIError{
		Message:   message,
		Type:      errType,
		Status:    status,
		RequestID: requestID,
	}
}

func extractErrorMessage(body any) string {
	m, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	keys := []string{"message", "error", "detail", "msg"}
	for _, key := range keys {
		if val, exists := m[key]; exists {
			if str := strings.TrimSpace(fmt.Sprintf("%v", val)); str != "" {
				return str
			}
		}
	}

	if wrapped, ok := m["error"].(map[string]any); ok {
		if val, exists := wrapped["message"]; exists {
			if str := strings.TrimSpace(fmt.Sprintf("%v", val)); str != "" {
				return str
			}
		}
	}

	return ""
}

func debugRequest(req *http.Request, body []byte) {
	fmt.Fprintf(os.Stderr, "--> %s %s\n", req.Method, sanitizeURL(req.URL))

	headerNames := make([]string, 0, len(req.Header))
	for name := range req.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range req.Header.Values(name) {
			fmt.Fprintf(os.Stderr, "--> %s: %s\n", name, sanitizeHeader(name, value))
		}
	}

	if len(body) > 0 {
		truncated := string(body)
		if len(truncated) > 1024 {
			truncated = truncated[:1024] + "..."
		}
		fmt.Fprintf(os.Stderr, "--> body: %s\n", truncated)
	}
}

func debugResponse(resp *http.Response, body []byte) {
	fmt.Fprintf(os.Stderr, "<-- %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

	headerNames := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range resp.Header.Values(name) {
			fmt.Fprintf(os.Stderr, "<-- %s: %s\n", name, sanitizeHeader(name, value))
		}
	}

	if len(body) > 0 {
		truncated := string(body)
		if len(truncated) > 1024 {
			truncated = truncated[:1024] + "..."
		}
		fmt.Fprintf(os.Stderr, "<-- body: %s\n", truncated)
	}
}

func sanitizeHeader(name, value string) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	if lowerName == "x-api-key" || lowerName == "authorization" {
		return "***REDACTED***"
	}
	return value
}

func sanitizeURL(u *url.URL) string {
	clone := *u
	query := clone.Query()
	for key := range query {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "api_key") || strings.Contains(lowerKey, "apikey") || strings.Contains(lowerKey, "auth") {
			query.Set(key, "***REDACTED***")
		}
	}
	clone.RawQuery = query.Encode()
	return clone.String()
}

func NewDefaultHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func detectMIMEType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
