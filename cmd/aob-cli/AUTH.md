# Authentication in AOB CLI

## Overview

The AOB CLI now includes a simple authentication system that stores your username locally and automatically includes it in all API requests via the `X-User-ID` header.

## Commands

### Login

```bash
# Login with username
aob login <username>

# Or prompt for username
aob login
```

This creates `~/.aob-cli/config.json` with your credentials:
```json
{
  "username": "your-username"
}
```

### Check Authentication Status

```bash
aob whoami
```

Shows:
- Current logged-in user
- Config file location
- Or "Not logged in" if no credentials found

### Logout

```bash
aob logout
```

Deletes the config file at `~/.aob-cli/config.json`.

## Automatic Authentication

When you run any API command (workflow, run, patch, etc.), the CLI will:

1. Check for existing credentials in `~/.aob-cli/config.json`
2. If found, use them automatically
3. If not found, prompt you to login:
   ```
   ⚠️  Not logged in. Please enter your credentials.

   Enter your username: _
   ```
4. After entering username, save it and continue with the command

**Example:**
```bash
# First time - prompts for login
$ aob workflow list
⚠️  Not logged in. Please enter your credentials.

Enter your username: alice

✓ Logged in as: alice
✓ Config saved to: /Users/you/.aob-cli/config.json

[Lists workflows...]

# Subsequent commands - uses saved credentials
$ aob workflow list
[Lists workflows immediately]
```

## How It Works

### Client Configuration

The `ApiClient` automatically includes the `X-User-ID` header in all requests:

```rust
// Initialized with username from config
let client = ApiClient::new(&api_url)?.with_username(config.username);

// All requests include: X-User-ID: alice
client.get("/api/v1/workflows").await?;
client.post("/api/v1/workflows", &body).await?;
client.delete("/api/v1/workflows/main").await?;
```

### Tag Namespacing

With the username header, the server implements tag namespacing:

- You create tag "main" → Stored as "alice/main"
- Another user creates "main" → Stored as "bob/main"
- You see only "main" in responses (prefix is hidden)
- Each user has their own namespace

**Example API Response:**
```json
{
  "tag": "main",
  "owner": "alice",
  "artifact_id": "uuid",
  "created_at": "2025-10-12T10:00:00Z"
}
```

## Config File Location

The config is stored at:
- **macOS/Linux**: `~/.aob-cli/config.json`
- **Windows**: `C:\Users\<username>\.aob-cli\config.json`

## Security Notes

**Current Implementation:**
- Username is stored in plain text
- No password required (for now)
- Config file has default permissions (readable by user only on Unix systems)

**Future Enhancements:**
- Add password/token authentication
- Implement OAuth/SSO support
- Add API key support
- Encrypt credentials at rest

## Examples

### Workflow Operations

```bash
# Login
$ aob login alice

# Create workflow (uses alice's namespace)
$ aob workflow create main workflow.json
✓ Created workflow: main (owner: alice)

# List your workflows
$ aob workflow list
main (owner: alice)
dev (owner: alice)

# Get workflow
$ aob workflow get main
{
  "tag": "main",
  "owner": "alice",
  ...
}
```

### Multi-User Scenario

```bash
# Alice's workflows
$ aob login alice
$ aob workflow create main workflow-v1.json

# Bob's workflows
$ aob logout
$ aob login bob
$ aob workflow create main workflow-v2.json

# Both users have "main" tag
# Alice sees: main (owner: alice)
# Bob sees: main (owner: bob)
```

## Troubleshooting

### "Not logged in" errors

If you get authentication errors:

1. Check if logged in:
   ```bash
   aob whoami
   ```

2. Try logging in again:
   ```bash
   aob login your-username
   ```

3. Check config file exists:
   ```bash
   cat ~/.aob-cli/config.json
   ```

### Config file issues

Delete and recreate:
```bash
rm ~/.aob-cli/config.json
aob login your-username
```

### X-User-ID header not sent

The CLI automatically includes this header. To verify, check API logs on the server side.

## Development

### Building

```bash
cd cmd/aob-cli
cargo build
```

### Testing Auth Flow

```bash
# Test commands
./target/debug/aob whoami          # Check status
./target/debug/aob login alice     # Login
./target/debug/aob whoami          # Verify
./target/debug/aob logout          # Logout
./target/debug/aob whoami          # Verify logout
```

### Code Structure

```
src/
├── auth/
│   └── mod.rs          # Config management, prompt_username(), ensure_authenticated()
├── client/
│   └── mod.rs          # ApiClient with X-User-ID header support
└── main.rs             # CLI commands: login, logout, whoami
```

## API Integration

The server expects the `X-User-ID` header in all requests:

```http
GET /api/v1/workflows HTTP/1.1
Host: localhost:8081
X-User-ID: alice
```

Without this header, the server returns:
```json
{
  "error": "authentication required (X-User-ID header missing)"
}
```

The middleware (`cmd/orchestrator/middleware/auth.go`) extracts and validates this header.
