package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// apiVersion is the Paperless-ngx REST API version we target.
const apiVersion = 5

// Client wraps the Paperless-ngx REST API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	upload  *http.Client // longer timeout for uploads/downloads
}

// NewClient creates a Paperless-ngx API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
		upload:  &http.Client{Timeout: 120 * time.Second},
	}
}

// WithToken returns a shallow copy of the client using a different API token.
// The underlying HTTP clients are shared.
func (c *Client) WithToken(token string) *Client {
	return &Client{
		baseURL: c.baseURL,
		token:   token,
		http:    c.http,
		upload:  c.upload,
	}
}

// CheckAPIVersion probes /api/ and warns if the Paperless-ngx instance
// exposes a newer API version than the one we target. This is advisory only —
// the server continues regardless.
func (c *Client) CheckAPIVersion() {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/", nil)
	if err != nil {
		log.Printf("warning: could not build API version check request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("warning: API version check failed (could not reach Paperless): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("warning: API version check returned HTTP %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("warning: API version check: could not read response: %v", err)
		return
	}

	// The /api/ root returns an object whose keys are endpoint names; the
	// response itself doesn't carry a top-level version field. However,
	// Paperless-ngx sets X-Version or we can check the header. The most
	// reliable signal is the X-Api-Version response header (integer).
	// Some builds also expose it in X-Version.
	if hdr := resp.Header.Get("X-Api-Version"); hdr != "" {
		if ver, err := strconv.Atoi(hdr); err == nil {
			if ver > apiVersion {
				log.Printf("WARNING: Paperless-ngx reports API version %d, but paperless-mcp targets version %d. "+
					"Newer API versions may have changed behaviour; please verify compatibility.", ver, apiVersion)
			} else {
				log.Printf("paperless-ngx API version: %d (target: %d) — OK", ver, apiVersion)
			}
			return
		}
	}

	// Fallback: look for a version key in the JSON body (some builds include it).
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err == nil {
		if verRaw, ok := root["version"]; ok {
			var ver int
			if json.Unmarshal(verRaw, &ver) == nil && ver > apiVersion {
				log.Printf("WARNING: Paperless-ngx reports API version %d, but paperless-mcp targets version %d. "+
					"Newer API versions may have changed behaviour; please verify compatibility.", ver, apiVersion)
				return
			}
		}
	}

	log.Printf("paperless-ngx API version check: could not determine remote version (targeting %d)", apiVersion)
}

// doRequest executes a request with the Paperless auth header and returns the body.
func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Token "+c.token)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", fmt.Sprintf("application/json; version=%d", apiVersion))
	}

	client := c.http
	if req.Method == http.MethodPost || req.ContentLength > 0 {
		client = c.upload
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("paperless API error %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return body, nil
}

// doGet performs a GET request with optional query parameters.
func (c *Client) doGet(path string, params url.Values) (json.RawMessage, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

// doPost performs a POST with a JSON body.
func (c *Client) doPost(path string, payload interface{}) (json.RawMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

// doPatch performs a PATCH with a JSON body.
func (c *Client) doPatch(path string, payload interface{}) (json.RawMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

// --- Document operations ---

// ListDocuments returns a paginated list of documents with optional filters.
func (c *Client) ListDocuments(page, pageSize int, ordering string, filters map[string]string) (json.RawMessage, error) {
	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}
	if ordering != "" {
		params.Set("ordering", ordering)
	}
	for k, v := range filters {
		params.Set(k, v)
	}
	return c.doGet("/api/documents/", params)
}

// SearchDocuments performs a full-text search across documents.
func (c *Client) SearchDocuments(query string, page, pageSize int) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("query", query)
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}
	return c.doGet("/api/documents/", params)
}

// GetDocument returns full details for a document by ID.
func (c *Client) GetDocument(id int) (json.RawMessage, error) {
	return c.doGet(fmt.Sprintf("/api/documents/%d/", id), nil)
}

// DownloadDocument downloads a document file and returns the bytes, filename, and content type.
func (c *Client) DownloadDocument(id int, original bool) ([]byte, string, string, error) {
	path := fmt.Sprintf("/api/documents/%d/download/", id)
	params := url.Values{}
	if original {
		params.Set("original", "true")
	}

	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.upload.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", "", fmt.Errorf("paperless API error %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("reading download: %w", err)
	}

	filename := "document"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		// Parse filename from Content-Disposition header.
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "filename=") {
				filename = strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
			}
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return body, filename, contentType, nil
}

// PostDocument uploads a new document via multipart form.
func (c *Client) PostDocument(filename string, fileData []byte, metadata map[string]interface{}) (json.RawMessage, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Add the file.
	part, err := w.CreateFormFile("document", filename)
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("writing file data: %w", err)
	}

	// Add metadata fields.
	for key, val := range metadata {
		switch v := val.(type) {
		case []interface{}:
			// Repeated fields (e.g. tags).
			for _, item := range v {
				if err := w.WriteField(key, fmt.Sprintf("%v", item)); err != nil {
					return nil, fmt.Errorf("writing field %s: %w", key, err)
				}
			}
		default:
			if err := w.WriteField(key, fmt.Sprintf("%v", v)); err != nil {
				return nil, fmt.Errorf("writing field %s: %w", key, err)
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/documents/post_document/", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

// UpdateDocument patches document metadata.
func (c *Client) UpdateDocument(id int, fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPatch(fmt.Sprintf("/api/documents/%d/", id), fields)
}

// BulkEditDocuments performs a bulk operation on multiple documents.
func (c *Client) BulkEditDocuments(documentIDs []int, method string, params map[string]interface{}) (json.RawMessage, error) {
	payload := make(map[string]interface{})
	for k, v := range params {
		payload[k] = v
	}
	payload["documents"] = documentIDs
	payload["method"] = method
	return c.doPost("/api/documents/bulk_edit/", payload)
}

// GetDocumentSuggestions returns auto-classification suggestions for a document.
func (c *Client) GetDocumentSuggestions(id int) (json.RawMessage, error) {
	return c.doGet(fmt.Sprintf("/api/documents/%d/suggestions/", id), nil)
}

// GetDocumentMetadata returns technical metadata for a document.
func (c *Client) GetDocumentMetadata(id int) (json.RawMessage, error) {
	return c.doGet(fmt.Sprintf("/api/documents/%d/metadata/", id), nil)
}

// --- Tags ---

func (c *Client) ListTags(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/tags/", params)
}

func (c *Client) CreateTag(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/tags/", fields)
}

// --- Correspondents ---

func (c *Client) ListCorrespondents(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/correspondents/", params)
}

func (c *Client) CreateCorrespondent(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/correspondents/", fields)
}

// --- Document types ---

func (c *Client) ListDocumentTypes(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/document_types/", params)
}

func (c *Client) CreateDocumentType(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/document_types/", fields)
}

// --- Storage paths ---

func (c *Client) ListStoragePaths(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/storage_paths/", params)
}

// --- Custom fields ---

func (c *Client) ListCustomFields(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/custom_fields/", params)
}

// --- Saved views ---

func (c *Client) ListSavedViews(params url.Values) (json.RawMessage, error) {
	params = withDefaultPageSize(params)
	return c.doGet("/api/saved_views/", params)
}

// withDefaultPageSize returns a copy of params with page_size defaulting to
// 1000 when not explicitly set. A nil input is treated as empty.
func withDefaultPageSize(params url.Values) url.Values {
	if params == nil {
		params = url.Values{}
	}
	if params.Get("page_size") == "" {
		params.Set("page_size", "1000")
	}
	return params
}

// truncate shortens a string for display in error messages.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
