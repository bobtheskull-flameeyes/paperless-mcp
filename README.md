# paperless-mcp

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server for
[Paperless-ngx](https://docs.paperless-ngx.com/). Lets LLM agents search, read,
tag, upload, and manage documents through a standard tool interface.

Written in Go with no external dependencies.

## Quick Start

```bash
# Build
go build -o paperless-mcp .

# Create config
cat > config.json <<'EOF'
{
  "paperless_url": "http://paperless.example.com:8000",
  "paperless_token": "your-paperless-api-token",
  "listen_addr": ":8035",
  "mcp_token": "your-mcp-bearer-token"
}
EOF

# Run
./paperless-mcp
```

## Configuration

Configuration is read from a JSON file (default `config.json`, override with
`-config path/to/file.json`).

| Field              | Description                                  | Default  |
|--------------------|----------------------------------------------|----------|
| `paperless_url`    | Paperless-ngx base URL                       | required |
| `paperless_token`  | Paperless-ngx API token                      | required |
| `listen_addr`      | Address to listen on                         | `:8035`  |
| `mcp_token`        | Bearer token for MCP endpoint authentication | (none)   |

### Environment Variable Overrides

Each config field has a corresponding environment variable that takes
precedence over the config file:

- `PAPERLESS_URL`
- `PAPERLESS_TOKEN`
- `LISTEN_ADDR`
- `MCP_TOKEN`

### Getting a Paperless-ngx API Token

1. Log into your Paperless-ngx instance
2. Click your username → "My Profile"
3. Under "Auth Token", click the refresh button to generate a new token

## Authentication

The MCP endpoint (`POST /mcp`) is protected by bearer token authentication
when `mcp_token` is configured. Clients must send:

```
Authorization: Bearer <mcp_token>
```

If `mcp_token` is empty, authentication is disabled (suitable for local
development only).

The server authenticates to Paperless-ngx using
`Authorization: Token <paperless_token>` on all API requests.

## API Endpoints

### `POST /mcp`

MCP Streamable HTTP transport. Accepts JSON-RPC 2.0 requests (single or
batch). Requires bearer authentication when configured.

### `GET /health`

Health check. Returns:

```json
{"status": "ok", "paperless_url": "http://paperless.example.com:8000"}
```

No authentication required.

## Available Tools

### Documents

| Tool                       | Description                                                    |
|----------------------------|----------------------------------------------------------------|
| `search_documents`         | Full-text search across all documents                          |
| `list_documents`           | List documents with filtering (by tag, correspondent, type, date, etc.) and sorting |
| `get_document`             | Get full details of a specific document                        |
| `download_document`        | Download file content (returns base64-encoded)                 |
| `upload_document`          | Upload a new document                                          |
| `update_document`          | Update document metadata (title, tags, correspondent, etc.)    |
| `bulk_edit_documents`      | Bulk operations: tag, retag, set correspondent/type, merge, split, rotate, delete |
| `get_document_suggestions` | Get auto-classification suggestions for a document             |
| `get_document_metadata`    | Get technical metadata (checksums, filenames, etc.)            |

### Metadata

| Tool                    | Description                  |
|-------------------------|------------------------------|
| `list_tags`             | List all tags                |
| `create_tag`            | Create a new tag             |
| `list_correspondents`   | List all correspondents      |
| `create_correspondent`  | Create a new correspondent   |
| `list_document_types`   | List all document types      |
| `create_document_type`  | Create a new document type   |
| `list_storage_paths`    | List all storage paths       |
| `list_custom_fields`    | List all custom fields       |
| `list_saved_views`      | List all saved views         |

## Command-Line Flags

```
-config string    path to config file (default "config.json")
-addr string      listen address (overrides config)
```

## License

0BSD
