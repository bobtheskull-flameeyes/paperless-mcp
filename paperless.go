package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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

// doRequest executes a request with the Paperless auth header and returns the body.
func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Token "+c.token)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json; version=5")
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

func (c *Client) ListTags() (json.RawMessage, error) {
	return c.doGet("/api/tags/", url.Values{"page_size": {"1000"}})
}

func (c *Client) CreateTag(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/tags/", fields)
}

// --- Correspondents ---

func (c *Client) ListCorrespondents() (json.RawMessage, error) {
	return c.doGet("/api/correspondents/", url.Values{"page_size": {"1000"}})
}

func (c *Client) CreateCorrespondent(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/correspondents/", fields)
}

// --- Document types ---

func (c *Client) ListDocumentTypes() (json.RawMessage, error) {
	return c.doGet("/api/document_types/", url.Values{"page_size": {"1000"}})
}

func (c *Client) CreateDocumentType(fields map[string]interface{}) (json.RawMessage, error) {
	return c.doPost("/api/document_types/", fields)
}

// --- Storage paths ---

func (c *Client) ListStoragePaths() (json.RawMessage, error) {
	return c.doGet("/api/storage_paths/", url.Values{"page_size": {"1000"}})
}

// --- Custom fields ---

func (c *Client) ListCustomFields() (json.RawMessage, error) {
	return c.doGet("/api/custom_fields/", url.Values{"page_size": {"1000"}})
}

// --- Saved views ---

func (c *Client) ListSavedViews() (json.RawMessage, error) {
	return c.doGet("/api/saved_views/", url.Values{"page_size": {"1000"}})
}

// truncate shortens a string for display in error messages.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
