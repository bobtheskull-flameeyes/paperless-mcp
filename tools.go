package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ToolRegistry holds all MCP tool definitions and dispatches calls.
type ToolRegistry struct {
	client *Client
	tools  []Tool
}

// NewToolRegistry creates the registry with all tool definitions.
func NewToolRegistry(client *Client) *ToolRegistry {
	r := &ToolRegistry{client: client}
	r.tools = r.defineTools()
	return r
}

// List returns the tools/list result.
func (r *ToolRegistry) List() ToolsListResult {
	return ToolsListResult{Tools: r.tools}
}

// Call dispatches a tool call by name.
func (r *ToolRegistry) Call(name string, args map[string]interface{}) (*ToolCallResult, error) {
	if args == nil {
		args = map[string]interface{}{}
	}

	switch name {
	case "search_documents":
		return r.searchDocuments(args), nil
	case "list_documents":
		return r.listDocuments(args), nil
	case "get_document":
		return r.getDocument(args), nil
	case "download_document":
		return r.downloadDocument(args), nil
	case "upload_document":
		return r.uploadDocument(args), nil
	case "update_document":
		return r.updateDocument(args), nil
	case "bulk_edit_documents":
		return r.bulkEditDocuments(args), nil
	case "get_document_suggestions":
		return r.getDocumentSuggestions(args), nil
	case "get_document_metadata":
		return r.getDocumentMetadata(args), nil
	case "list_tags":
		return r.listTags(args), nil
	case "create_tag":
		return r.createTag(args), nil
	case "list_correspondents":
		return r.listCorrespondents(args), nil
	case "create_correspondent":
		return r.createCorrespondent(args), nil
	case "list_document_types":
		return r.listDocumentTypes(args), nil
	case "create_document_type":
		return r.createDocumentType(args), nil
	case "list_storage_paths":
		return r.listStoragePaths(args), nil
	case "list_custom_fields":
		return r.listCustomFields(args), nil
	case "list_saved_views":
		return r.listSavedViews(args), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// --- Argument extraction helpers ---

func getStringArg(args map[string]interface{}, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getIntArg(args map[string]interface{}, key string) (int, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case int:
		return n, true
	default:
		// Try parsing string.
		s, ok := v.(string)
		if ok {
			i, err := strconv.Atoi(s)
			if err == nil {
				return i, true
			}
		}
		return 0, false
	}
}

func getBoolArg(args map[string]interface{}, key string) (bool, bool) {
	v, ok := args[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func getIntArrayArg(args map[string]interface{}, key string) ([]int, bool) {
	v, ok := args[key]
	if !ok {
		return nil, false
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	result := make([]int, 0, len(arr))
	for _, item := range arr {
		switch n := item.(type) {
		case float64:
			result = append(result, int(n))
		case json.Number:
			i, err := n.Int64()
			if err != nil {
				return nil, false
			}
			result = append(result, int(i))
		default:
			return nil, false
		}
	}
	return result, true
}

// extractNameFilters pulls common name/id filter params, pagination, and
// ordering from tool call args into url.Values. Used by list_tags,
// list_correspondents, list_document_types, list_storage_paths, and
// list_custom_fields.
func extractNameFilters(args map[string]interface{}) url.Values {
	params := url.Values{}
	stringKeys := []string{
		"name__icontains", "name__istartswith", "name__iexact", "name__iendswith",
		"ordering",
	}
	for _, key := range stringKeys {
		if v, ok := getStringArg(args, key); ok {
			params.Set(key, v)
		}
	}
	intKeys := []string{"page", "page_size"}
	for _, key := range intKeys {
		if v, ok := getIntArg(args, key); ok {
			params.Set(key, strconv.Itoa(v))
		}
	}
	return params
}

// --- Tool handlers ---

func (r *ToolRegistry) searchDocuments(args map[string]interface{}) *ToolCallResult {
	query, ok := getStringArg(args, "query")
	if !ok || query == "" {
		return ErrorResult("required parameter 'query' is missing")
	}
	page, _ := getIntArg(args, "page")
	pageSize, _ := getIntArg(args, "page_size")

	data, err := r.client.SearchDocuments(query, page, pageSize)
	if err != nil {
		return ErrorResult("search failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listDocuments(args map[string]interface{}) *ToolCallResult {
	page, _ := getIntArg(args, "page")
	pageSize, _ := getIntArg(args, "page_size")
	ordering, _ := getStringArg(args, "ordering")

	filters := map[string]string{}
	filterKeys := []string{
		"tags__id__in", "tags__id__all", "tags__id__none",
		"correspondent__id", "document_type__id",
		"storage_path__id", "is_tagged",
		"created__date__gt", "created__date__lt",
		"created__date__gte", "created__date__lte",
		"added__date__gt", "added__date__lt",
		"added__date__gte", "added__date__lte",
		"title__icontains", "content__icontains",
		"mime_type",
	}
	for _, key := range filterKeys {
		if v, ok := getStringArg(args, key); ok {
			filters[key] = v
		}
		// Also accept integer values for integer filter fields.
		if _, ok := filters[key]; !ok {
			if n, ok := getIntArg(args, key); ok {
				filters[key] = strconv.Itoa(n)
			}
		}
	}

	// full_perms and fields are Paperless query parameters rather than
	// Django-style filters, so we inject them directly.
	if v, ok := getBoolArg(args, "full_perms"); ok && v {
		filters["full_perms"] = "true"
	}
	if v, ok := getStringArg(args, "fields"); ok {
		filters["fields"] = v
	}

	data, err := r.client.ListDocuments(page, pageSize, ordering, filters)
	if err != nil {
		return ErrorResult("list documents failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) getDocument(args map[string]interface{}) *ToolCallResult {
	id, ok := getIntArg(args, "id")
	if !ok {
		return ErrorResult("required parameter 'id' is missing")
	}

	data, err := r.client.GetDocument(id)
	if err != nil {
		return ErrorResult("get document failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) downloadDocument(args map[string]interface{}) *ToolCallResult {
	id, ok := getIntArg(args, "id")
	if !ok {
		return ErrorResult("required parameter 'id' is missing")
	}
	original, _ := getBoolArg(args, "original")

	fileData, filename, contentType, err := r.client.DownloadDocument(id, original)
	if err != nil {
		return ErrorResult("download failed: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(fileData)
	sizeKB := float64(len(fileData)) / 1024.0

	meta := fmt.Sprintf("Filename: %s\nContent-Type: %s\nSize: %.1f KB", filename, contentType, sizeKB)

	return &ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: meta},
			{Type: "text", Text: encoded},
		},
	}
}

func (r *ToolRegistry) uploadDocument(args map[string]interface{}) *ToolCallResult {
	fileB64, ok := getStringArg(args, "file")
	if !ok || fileB64 == "" {
		return ErrorResult("required parameter 'file' (base64) is missing")
	}
	filename, ok := getStringArg(args, "filename")
	if !ok || filename == "" {
		return ErrorResult("required parameter 'filename' is missing")
	}

	fileData, err := base64.StdEncoding.DecodeString(fileB64)
	if err != nil {
		return ErrorResult("invalid base64 in 'file': %v", err)
	}

	metadata := map[string]interface{}{}
	if v, ok := getStringArg(args, "title"); ok {
		metadata["title"] = v
	}
	if v, ok := getStringArg(args, "created"); ok {
		metadata["created"] = v
	}
	if v, ok := getIntArg(args, "correspondent"); ok {
		metadata["correspondent"] = v
	}
	if v, ok := getIntArg(args, "document_type"); ok {
		metadata["document_type"] = v
	}
	if v, ok := getStringArg(args, "archive_serial_number"); ok {
		metadata["archive_serial_number"] = v
	}
	if tags, ok := getIntArrayArg(args, "tags"); ok {
		ifaces := make([]interface{}, len(tags))
		for i, t := range tags {
			ifaces[i] = t
		}
		metadata["tags"] = ifaces
	}

	data, err := r.client.PostDocument(filename, fileData, metadata)
	if err != nil {
		return ErrorResult("upload failed: %v", err)
	}
	if len(data) == 0 {
		return TextResult("Document uploaded successfully (queued for processing)")
	}
	return JSONResult(data)
}

func (r *ToolRegistry) updateDocument(args map[string]interface{}) *ToolCallResult {
	id, ok := getIntArg(args, "id")
	if !ok {
		return ErrorResult("required parameter 'id' is missing")
	}

	fields := map[string]interface{}{}
	if v, ok := getStringArg(args, "title"); ok {
		fields["title"] = v
	}
	if v, ok := getIntArg(args, "correspondent"); ok {
		fields["correspondent"] = v
	}
	if v, ok := getIntArg(args, "document_type"); ok {
		fields["document_type"] = v
	}
	if v, ok := getIntArg(args, "archive_serial_number"); ok {
		fields["archive_serial_number"] = v
	}
	if tags, ok := getIntArrayArg(args, "tags"); ok {
		fields["tags"] = tags
	}
	if cf, ok := args["custom_fields"]; ok {
		fields["custom_fields"] = cf
	}

	if len(fields) == 0 {
		return ErrorResult("no fields to update")
	}

	data, err := r.client.UpdateDocument(id, fields)
	if err != nil {
		return ErrorResult("update failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) bulkEditDocuments(args map[string]interface{}) *ToolCallResult {
	docIDs, ok := getIntArrayArg(args, "documents")
	if !ok || len(docIDs) == 0 {
		return ErrorResult("required parameter 'documents' (array of IDs) is missing")
	}
	method, ok := getStringArg(args, "method")
	if !ok || method == "" {
		return ErrorResult("required parameter 'method' is missing")
	}

	params := map[string]interface{}{}
	// Copy relevant params based on method.
	paramKeys := []string{
		"correspondent", "document_type", "storage_path", "tag",
		"metadata_document_id", "degrees",
	}
	for _, key := range paramKeys {
		if v, ok := getIntArg(args, key); ok {
			params[key] = v
		}
	}
	if v, ok := getBoolArg(args, "delete_originals"); ok {
		params["delete_originals"] = v
	}
	if v, ok := getStringArg(args, "pages"); ok {
		params["pages"] = v
	}
	if v, ok := getIntArrayArg(args, "add_tags"); ok {
		params["add_tags"] = v
	}
	if v, ok := getIntArrayArg(args, "remove_tags"); ok {
		params["remove_tags"] = v
	}

	data, err := r.client.BulkEditDocuments(docIDs, method, params)
	if err != nil {
		return ErrorResult("bulk edit failed: %v", err)
	}
	if len(data) == 0 {
		return TextResult("Bulk edit completed successfully")
	}
	return JSONResult(data)
}

func (r *ToolRegistry) getDocumentSuggestions(args map[string]interface{}) *ToolCallResult {
	id, ok := getIntArg(args, "id")
	if !ok {
		return ErrorResult("required parameter 'id' is missing")
	}

	data, err := r.client.GetDocumentSuggestions(id)
	if err != nil {
		return ErrorResult("get suggestions failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) getDocumentMetadata(args map[string]interface{}) *ToolCallResult {
	id, ok := getIntArg(args, "id")
	if !ok {
		return ErrorResult("required parameter 'id' is missing")
	}

	data, err := r.client.GetDocumentMetadata(id)
	if err != nil {
		return ErrorResult("get metadata failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listTags(args map[string]interface{}) *ToolCallResult {
	params := extractNameFilters(args)
	if v, ok := getBoolArg(args, "is_root"); ok {
		if v {
			params.Set("is_root", "true")
		} else {
			params.Set("is_root", "false")
		}
	}
	data, err := r.client.ListTags(params)
	if err != nil {
		return ErrorResult("list tags failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) createTag(args map[string]interface{}) *ToolCallResult {
	name, ok := getStringArg(args, "name")
	if !ok || name == "" {
		return ErrorResult("required parameter 'name' is missing")
	}

	fields := map[string]interface{}{"name": name}
	if v, ok := getStringArg(args, "color"); ok {
		fields["color"] = v
	}
	if v, ok := getStringArg(args, "match"); ok {
		fields["match"] = v
	}
	if v, ok := getIntArg(args, "matching_algorithm"); ok {
		fields["matching_algorithm"] = v
	}
	if v, ok := getBoolArg(args, "is_insensitive"); ok {
		fields["is_insensitive"] = v
	}

	data, err := r.client.CreateTag(fields)
	if err != nil {
		return ErrorResult("create tag failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listCorrespondents(args map[string]interface{}) *ToolCallResult {
	params := extractNameFilters(args)
	data, err := r.client.ListCorrespondents(params)
	if err != nil {
		return ErrorResult("list correspondents failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) createCorrespondent(args map[string]interface{}) *ToolCallResult {
	name, ok := getStringArg(args, "name")
	if !ok || name == "" {
		return ErrorResult("required parameter 'name' is missing")
	}

	fields := map[string]interface{}{"name": name}
	if v, ok := getStringArg(args, "match"); ok {
		fields["match"] = v
	}
	if v, ok := getIntArg(args, "matching_algorithm"); ok {
		fields["matching_algorithm"] = v
	}
	if v, ok := getBoolArg(args, "is_insensitive"); ok {
		fields["is_insensitive"] = v
	}

	data, err := r.client.CreateCorrespondent(fields)
	if err != nil {
		return ErrorResult("create correspondent failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listDocumentTypes(args map[string]interface{}) *ToolCallResult {
	params := extractNameFilters(args)
	data, err := r.client.ListDocumentTypes(params)
	if err != nil {
		return ErrorResult("list document types failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) createDocumentType(args map[string]interface{}) *ToolCallResult {
	name, ok := getStringArg(args, "name")
	if !ok || name == "" {
		return ErrorResult("required parameter 'name' is missing")
	}

	fields := map[string]interface{}{"name": name}
	if v, ok := getStringArg(args, "match"); ok {
		fields["match"] = v
	}
	if v, ok := getIntArg(args, "matching_algorithm"); ok {
		fields["matching_algorithm"] = v
	}
	if v, ok := getBoolArg(args, "is_insensitive"); ok {
		fields["is_insensitive"] = v
	}

	data, err := r.client.CreateDocumentType(fields)
	if err != nil {
		return ErrorResult("create document type failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listStoragePaths(args map[string]interface{}) *ToolCallResult {
	params := extractNameFilters(args)
	// Storage paths also support filtering on the path field itself.
	for _, key := range []string{"path__icontains", "path__istartswith", "path__iexact", "path__iendswith"} {
		if v, ok := getStringArg(args, key); ok {
			params.Set(key, v)
		}
	}
	data, err := r.client.ListStoragePaths(params)
	if err != nil {
		return ErrorResult("list storage paths failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listCustomFields(args map[string]interface{}) *ToolCallResult {
	params := extractNameFilters(args)
	data, err := r.client.ListCustomFields(params)
	if err != nil {
		return ErrorResult("list custom fields failed: %v", err)
	}
	return JSONResult(data)
}

func (r *ToolRegistry) listSavedViews(args map[string]interface{}) *ToolCallResult {
	params := url.Values{}
	if v, ok := getIntArg(args, "page"); ok {
		params.Set("page", strconv.Itoa(v))
	}
	if v, ok := getIntArg(args, "page_size"); ok {
		params.Set("page_size", strconv.Itoa(v))
	}
	if v, ok := getStringArg(args, "ordering"); ok {
		params.Set("ordering", v)
	}
	data, err := r.client.ListSavedViews(params)
	if err != nil {
		return ErrorResult("list saved views failed: %v", err)
	}
	return JSONResult(data)
}

// --- Tool definitions ---

func (r *ToolRegistry) defineTools() []Tool {
	return []Tool{
		{
			Name:        "search_documents",
			Description: "Full-text search across all documents in Paperless-ngx.",
			InputSchema: jsonSchema(map[string]propDef{
				"query":     {Type: "string", Desc: "Search query string."},
				"page":      {Type: "integer", Desc: "Page number (default 1)."},
				"page_size": {Type: "integer", Desc: "Results per page (default 25)."},
			}, []string{"query"}),
		},
		{
			Name:        "list_documents",
			Description: "List documents with optional filtering and sorting. The response includes 'count' (total matches), 'next'/'previous' (pagination URLs — use 'page' parameter instead of following these directly), and an 'all' field containing every matching document ID in one shot (useful for getting the full ID set without paginating). Note: pagination URLs returned by Paperless may use http:// even when the server is behind https — always use the 'page' parameter to paginate.",
			InputSchema: jsonSchema(map[string]propDef{
				"page":                {Type: "integer", Desc: "Page number."},
				"page_size":           {Type: "integer", Desc: "Results per page."},
				"ordering":            {Type: "string", Desc: "Sort field, e.g. '-created', 'title', '-added'."},
				"tags__id__in":        {Type: "string", Desc: "Comma-separated tag IDs — matches documents with ANY of these tags (OR)."},
				"tags__id__all":       {Type: "string", Desc: "Comma-separated tag IDs — matches documents with ALL of these tags (AND)."},
				"tags__id__none":      {Type: "string", Desc: "Comma-separated tag IDs — excludes documents with any of these tags."},
				"correspondent__id":   {Type: "integer", Desc: "Filter by correspondent ID."},
				"document_type__id":   {Type: "integer", Desc: "Filter by document type ID."},
				"storage_path__id":    {Type: "integer", Desc: "Filter by storage path ID."},
				"is_tagged":           {Type: "string", Desc: "Filter: 'true' for tagged, 'false' for untagged."},
				"created__date__gt":   {Type: "string", Desc: "Filter: created after date, exclusive (YYYY-MM-DD)."},
				"created__date__lt":   {Type: "string", Desc: "Filter: created before date, exclusive (YYYY-MM-DD)."},
				"created__date__gte":  {Type: "string", Desc: "Filter: created on or after date, inclusive (YYYY-MM-DD)."},
				"created__date__lte":  {Type: "string", Desc: "Filter: created on or before date, inclusive (YYYY-MM-DD)."},
				"added__date__gt":     {Type: "string", Desc: "Filter: added after date, exclusive (YYYY-MM-DD)."},
				"added__date__lt":     {Type: "string", Desc: "Filter: added before date, exclusive (YYYY-MM-DD)."},
				"added__date__gte":    {Type: "string", Desc: "Filter: added on or after date, inclusive (YYYY-MM-DD)."},
				"added__date__lte":    {Type: "string", Desc: "Filter: added on or before date, inclusive (YYYY-MM-DD)."},
				"title__icontains":    {Type: "string", Desc: "Filter: title contains (case-insensitive)."},
				"content__icontains":  {Type: "string", Desc: "Filter: content contains (case-insensitive)."},
				"mime_type":           {Type: "string", Desc: "Filter by MIME type (e.g. 'application/pdf')."},
				"full_perms":          {Type: "boolean", Desc: "Include full permission objects in results (default false)."},
				"fields":              {Type: "string", Desc: "Comma-separated list of fields to return (e.g. 'id,title' for lightweight queries)."},
			}, nil),
		},
		{
			Name:        "get_document",
			Description: "Get full details of a specific document by ID.",
			InputSchema: jsonSchema(map[string]propDef{
				"id": {Type: "integer", Desc: "Document ID."},
			}, []string{"id"}),
		},
		{
			Name:        "download_document",
			Description: "Download the file content of a document. Returns metadata and base64-encoded file content.",
			InputSchema: jsonSchema(map[string]propDef{
				"id":       {Type: "integer", Desc: "Document ID."},
				"original": {Type: "boolean", Desc: "If true, download original file instead of archived version."},
			}, []string{"id"}),
		},
		{
			Name:        "upload_document",
			Description: "Upload a new document to Paperless-ngx. Note: to set permissions on creation, use the key 'set_permissions' (not 'permissions').",
			InputSchema: jsonSchema(map[string]propDef{
				"file":                   {Type: "string", Desc: "Base64-encoded file content."},
				"filename":               {Type: "string", Desc: "Filename including extension (e.g. 'invoice.pdf')."},
				"title":                  {Type: "string", Desc: "Document title."},
				"created":                {Type: "string", Desc: "Created date (ISO format, e.g. '2024-01-19')."},
				"correspondent":          {Type: "integer", Desc: "Correspondent ID."},
				"document_type":          {Type: "integer", Desc: "Document type ID."},
				"tags":                   {Type: "array", Desc: "Array of tag IDs.", Items: &itemsDef{Type: "integer"}},
				"archive_serial_number":  {Type: "string", Desc: "Archive serial number."},
			}, []string{"file", "filename"}),
		},
		{
			Name:        "update_document",
			Description: "Update metadata of an existing document. Note: to set permissions, use the key 'set_permissions' (not 'permissions'). GET returns 'permissions' but PATCH expects 'set_permissions'.",
			InputSchema: jsonSchema(map[string]propDef{
				"id":                    {Type: "integer", Desc: "Document ID."},
				"title":                 {Type: "string", Desc: "New title."},
				"correspondent":         {Type: "integer", Desc: "Correspondent ID."},
				"document_type":         {Type: "integer", Desc: "Document type ID."},
				"tags":                  {Type: "array", Desc: "Array of tag IDs (replaces existing).", Items: &itemsDef{Type: "integer"}},
				"archive_serial_number": {Type: "integer", Desc: "Archive serial number."},
				"custom_fields":         {Type: "array", Desc: "Array of {field: int, value: any} objects.", Items: &itemsDef{Type: "object"}},
			}, []string{"id"}),
		},
		{
			Name:        "bulk_edit_documents",
			Description: "Perform bulk operations on multiple documents. Methods: set_correspondent, set_document_type, set_storage_path, add_tag, remove_tag, modify_tags, delete, reprocess, merge, split, rotate, delete_pages.",
			InputSchema: jsonSchema(map[string]propDef{
				"documents":            {Type: "array", Desc: "Array of document IDs.", Items: &itemsDef{Type: "integer"}},
				"method":               {Type: "string", Desc: "Bulk edit method."},
				"correspondent":        {Type: "integer", Desc: "Correspondent ID (for set_correspondent)."},
				"document_type":        {Type: "integer", Desc: "Document type ID (for set_document_type)."},
				"storage_path":         {Type: "integer", Desc: "Storage path ID (for set_storage_path)."},
				"tag":                  {Type: "integer", Desc: "Tag ID (for add_tag/remove_tag)."},
				"add_tags":             {Type: "array", Desc: "Tag IDs to add (for modify_tags).", Items: &itemsDef{Type: "integer"}},
				"remove_tags":          {Type: "array", Desc: "Tag IDs to remove (for modify_tags).", Items: &itemsDef{Type: "integer"}},
				"metadata_document_id": {Type: "integer", Desc: "Document ID for merge metadata source."},
				"delete_originals":     {Type: "boolean", Desc: "Delete originals after merge/split."},
				"pages":                {Type: "string", Desc: "Page specification for split/delete_pages, e.g. '[1,2-3,4]'."},
				"degrees":              {Type: "integer", Desc: "Rotation degrees (90, 180, 270)."},
			}, []string{"documents", "method"}),
		},
		{
			Name:        "get_document_suggestions",
			Description: "Get auto-classification suggestions (tags, correspondent, document type) for a document.",
			InputSchema: jsonSchema(map[string]propDef{
				"id": {Type: "integer", Desc: "Document ID."},
			}, []string{"id"}),
		},
		{
			Name:        "get_document_metadata",
			Description: "Get technical metadata (checksums, media filename, original filename, etc.) for a document.",
			InputSchema: jsonSchema(map[string]propDef{
				"id": {Type: "integer", Desc: "Document ID."},
			}, []string{"id"}),
		},
		{
			Name:        "list_tags",
			Description: "List tags with optional filtering. Returns all tags when called without filters.",
			InputSchema: jsonSchema(map[string]propDef{
				"name__icontains":  {Type: "string", Desc: "Filter: name contains (case-insensitive)."},
				"name__istartswith": {Type: "string", Desc: "Filter: name starts with (case-insensitive)."},
				"name__iexact":     {Type: "string", Desc: "Filter: name matches exactly (case-insensitive)."},
				"name__iendswith":  {Type: "string", Desc: "Filter: name ends with (case-insensitive)."},
				"is_root":         {Type: "boolean", Desc: "Filter: true for root tags (no parent), false for child tags."},
				"ordering":        {Type: "string", Desc: "Sort field, e.g. 'name', '-name', 'document_count', '-document_count'."},
				"page":            {Type: "integer", Desc: "Page number (default 1)."},
				"page_size":       {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
		{
			Name:        "create_tag",
			Description: "Create a new tag. matching_algorithm: 1=Any, 2=All, 3=Exact, 4=RegEx, 5=Fuzzy, 6=Auto.",
			InputSchema: jsonSchema(map[string]propDef{
				"name":               {Type: "string", Desc: "Tag name."},
				"color":              {Type: "string", Desc: "Hex colour code (e.g. '#ff0000')."},
				"match":              {Type: "string", Desc: "Text pattern for auto-matching."},
				"matching_algorithm": {Type: "integer", Desc: "Matching algorithm (1-6)."},
				"is_insensitive":     {Type: "boolean", Desc: "Case-insensitive matching."},
			}, []string{"name"}),
		},
		{
			Name:        "list_correspondents",
			Description: "List correspondents with optional filtering. Returns all correspondents when called without filters.",
			InputSchema: jsonSchema(map[string]propDef{
				"name__icontains":  {Type: "string", Desc: "Filter: name contains (case-insensitive)."},
				"name__istartswith": {Type: "string", Desc: "Filter: name starts with (case-insensitive)."},
				"name__iexact":     {Type: "string", Desc: "Filter: name matches exactly (case-insensitive)."},
				"name__iendswith":  {Type: "string", Desc: "Filter: name ends with (case-insensitive)."},
				"ordering":        {Type: "string", Desc: "Sort field, e.g. 'name', '-name', 'document_count', '-document_count', 'last_correspondence'."},
				"page":            {Type: "integer", Desc: "Page number (default 1)."},
				"page_size":       {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
		{
			Name:        "create_correspondent",
			Description: "Create a new correspondent. matching_algorithm: 1=Any, 2=All, 3=Exact, 4=RegEx, 5=Fuzzy, 6=Auto.",
			InputSchema: jsonSchema(map[string]propDef{
				"name":               {Type: "string", Desc: "Correspondent name."},
				"match":              {Type: "string", Desc: "Text pattern for auto-matching."},
				"matching_algorithm": {Type: "integer", Desc: "Matching algorithm (1-6)."},
				"is_insensitive":     {Type: "boolean", Desc: "Case-insensitive matching."},
			}, []string{"name"}),
		},
		{
			Name:        "list_document_types",
			Description: "List document types with optional filtering. Returns all document types when called without filters.",
			InputSchema: jsonSchema(map[string]propDef{
				"name__icontains":  {Type: "string", Desc: "Filter: name contains (case-insensitive)."},
				"name__istartswith": {Type: "string", Desc: "Filter: name starts with (case-insensitive)."},
				"name__iexact":     {Type: "string", Desc: "Filter: name matches exactly (case-insensitive)."},
				"name__iendswith":  {Type: "string", Desc: "Filter: name ends with (case-insensitive)."},
				"ordering":        {Type: "string", Desc: "Sort field, e.g. 'name', '-name', 'document_count', '-document_count'."},
				"page":            {Type: "integer", Desc: "Page number (default 1)."},
				"page_size":       {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
		{
			Name:        "create_document_type",
			Description: "Create a new document type. matching_algorithm: 1=Any, 2=All, 3=Exact, 4=RegEx, 5=Fuzzy, 6=Auto.",
			InputSchema: jsonSchema(map[string]propDef{
				"name":               {Type: "string", Desc: "Document type name."},
				"match":              {Type: "string", Desc: "Text pattern for auto-matching."},
				"matching_algorithm": {Type: "integer", Desc: "Matching algorithm (1-6)."},
				"is_insensitive":     {Type: "boolean", Desc: "Case-insensitive matching."},
			}, []string{"name"}),
		},
		{
			Name:        "list_storage_paths",
			Description: "List storage paths with optional filtering. Returns all storage paths when called without filters.",
			InputSchema: jsonSchema(map[string]propDef{
				"name__icontains":  {Type: "string", Desc: "Filter: name contains (case-insensitive)."},
				"name__istartswith": {Type: "string", Desc: "Filter: name starts with (case-insensitive)."},
				"name__iexact":     {Type: "string", Desc: "Filter: name matches exactly (case-insensitive)."},
				"name__iendswith":  {Type: "string", Desc: "Filter: name ends with (case-insensitive)."},
				"path__icontains":  {Type: "string", Desc: "Filter: path template contains (case-insensitive)."},
				"path__istartswith": {Type: "string", Desc: "Filter: path template starts with (case-insensitive)."},
				"path__iexact":     {Type: "string", Desc: "Filter: path template matches exactly (case-insensitive)."},
				"path__iendswith":  {Type: "string", Desc: "Filter: path template ends with (case-insensitive)."},
				"ordering":        {Type: "string", Desc: "Sort field, e.g. 'name', '-name', 'path', '-path', 'document_count'."},
				"page":            {Type: "integer", Desc: "Page number (default 1)."},
				"page_size":       {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
		{
			Name:        "list_custom_fields",
			Description: "List custom field definitions with optional filtering. Returns all custom fields when called without filters.",
			InputSchema: jsonSchema(map[string]propDef{
				"name__icontains":  {Type: "string", Desc: "Filter: name contains (case-insensitive)."},
				"name__istartswith": {Type: "string", Desc: "Filter: name starts with (case-insensitive)."},
				"name__iexact":     {Type: "string", Desc: "Filter: name matches exactly (case-insensitive)."},
				"name__iendswith":  {Type: "string", Desc: "Filter: name ends with (case-insensitive)."},
				"ordering":        {Type: "string", Desc: "Sort field."},
				"page":            {Type: "integer", Desc: "Page number (default 1)."},
				"page_size":       {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
		{
			Name:        "list_saved_views",
			Description: "List saved views with optional pagination.",
			InputSchema: jsonSchema(map[string]propDef{
				"ordering":  {Type: "string", Desc: "Sort field."},
				"page":      {Type: "integer", Desc: "Page number (default 1)."},
				"page_size": {Type: "integer", Desc: "Results per page (default 1000)."},
			}, nil),
		},
	}
}

// --- JSON Schema helpers ---

type propDef struct {
	Type  string
	Desc  string
	Items *itemsDef
}

type itemsDef struct {
	Type string
}

func jsonSchema(props map[string]propDef, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type": "object",
	}

	if len(props) > 0 {
		properties := map[string]interface{}{}
		for name, def := range props {
			prop := map[string]interface{}{
				"type":        def.Type,
				"description": def.Desc,
			}
			if def.Items != nil {
				prop["items"] = map[string]interface{}{"type": def.Items.Type}
			}
			properties[name] = prop
		}
		schema["properties"] = properties
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

