# paperless-mcp

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server for
[Paperless-ngx](https://docs.paperless-ngx.com/). Lets LLM agents search, read,
tag, upload, and manage documents through a standard tool interface.

Written in Go with no external dependencies.

## Quick Start

### Binary

```bash
go build -o paperless-mcp .

cat > config.json <<'EOF'
{
  "paperless_url": "http://paperless.example.com:8000",
  "paperless_token": "your-paperless-api-token",
  "listen_addr": ":8035",
  "mcp_token": "your-mcp-bearer-token"
}
EOF

./paperless-mcp
```

### Docker

```bash
# Build
docker build -t paperless-mcp .

# Run
docker run -d --name paperless-mcp \
  -p 8035:8035 \
  -e PAPERLESS_URL=http://paperless.example.com:8000 \
  -e PAPERLESS_TOKEN=your-paperless-api-token \
  -e MCP_TOKEN=your-mcp-bearer-token \
  paperless-mcp
```

Environment variables are the preferred configuration method for Docker
deployments. You can also mount a config file:

```bash
docker run -d --name paperless-mcp \
  -p 8035:8035 \
  -v /path/to/config.json:/etc/paperless-mcp/config.json \
  paperless-mcp -config /etc/paperless-mcp/config.json
```

### Docker Compose (with Paperless-ngx)

An example compose file is included that runs the MCP server alongside a
full Paperless-ngx stack (Postgres, Redis, webserver):

```bash
cp docker-compose.example.yml docker-compose.yml
# Edit environment variables in docker-compose.yml
docker compose up -d
```

The MCP server connects to Paperless-ngx via Docker networking
(`http://webserver:8000`), so no port exposure is needed for the Paperless
API — only the MCP endpoint is published.

## Configuration

Configuration is read from a JSON file (default `config.json`, override with
`-config path/to/file.json`). Environment variables take precedence over the
config file.

| Field / Env Var      | Description                                  | Default  |
|----------------------|----------------------------------------------|----------|
| `paperless_url` / `PAPERLESS_URL`     | Paperless-ngx base URL        | required |
| `paperless_token` / `PAPERLESS_TOKEN` | Paperless-ngx API token       | required |
| `listen_addr` / `LISTEN_ADDR`         | Address to listen on          | `:8035`  |
| `mcp_token` / `MCP_TOKEN`             | Bearer token for MCP endpoint | (none)   |

Set `mcp_token` to the special value `"paperless"` to reuse the Paperless-ngx
API token for MCP authentication. This lets you manage a single token through
the Paperless UI (My Profile → Auth Token) instead of maintaining a separate
secret on the MCP server.

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

## API Compatibility

The server targets **Paperless-ngx REST API version 5**. At startup it probes
the `/api/` endpoint and logs a warning if the instance reports a newer version,
so you notice when an upgrade might have introduced breaking changes. The server
does not refuse to start — the warning is advisory only.

### Known Paperless-ngx API quirks

- **Pagination URLs use HTTP even when Paperless is behind HTTPS.** The `next`
  and `previous` fields in list responses may have an `http://` scheme when the
  actual endpoint requires `https://`. Always use the `page` parameter to
  paginate rather than following `next`/`previous` URLs directly.
- **`permissions` vs `set_permissions`.** GET responses include a `permissions`
  field, but POST/PATCH requests must use `set_permissions` instead. The tool
  descriptions note this, but callers should be aware.
- **The `all` field in list responses.** List endpoints return an `all` array
  containing every matching document ID. This lets you get the full ID set
  without paginating, which avoids skew when documents are being modified
  concurrently.

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

## Install from Source

```bash
go install github.com/bobtheskull-flameeyes/paperless-mcp@latest
```

## License

0BSD
