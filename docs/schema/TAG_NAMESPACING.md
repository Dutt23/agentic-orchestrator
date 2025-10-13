# Tag Namespacing

## Overview

Tag namespacing allows multiple users to create tags with the same name by automatically prefixing tag names with usernames internally, while maintaining a clean user experience.

## Problem Solved

**Before namespacing:**
- Tags were globally unique
- If Alice creates "main", Bob cannot create "main"
- Conflicting names force awkward conventions like "alice-main", "bob-main"

**With namespacing:**
- Alice creates "main" → Stored as "alice/main"
- Bob creates "main" → Stored as "bob/main"
- Both users see clean name "main" in their interface
- No conflicts, clean UX

## How It Works

### Internal Storage

Tags are stored with username prefixes in the database:

```sql
-- tag table
┌─────────────────┬─────────────┬───────────┐
│ tag_name (PK)   │ target_kind │ target_id │
├─────────────────┼─────────────┼───────────┤
│ alice/main      │ patch_set   │ P5        │  ← Alice's main
│ bob/main        │ dag_version │ V2        │  ← Bob's main
│ alice/feature-1 │ patch_set   │ P10       │  ← Alice's feature
│ _global_/prod   │ dag_version │ V1        │  ← Global tag
└─────────────────┴─────────────┴───────────┘
```

### User Experience

Users only see clean tag names:

```bash
# Alice creates "main"
POST /api/v1/workflows
Headers: X-User-ID: alice
Body: {"tag_name": "main", ...}

# Response (clean name shown)
{"tag": "main", "owner": "alice", ...}

# Bob creates "main"
POST /api/v1/workflows
Headers: X-User-ID: bob
Body: {"tag_name": "main", ...}

# Response (clean name shown)
{"tag": "main", "owner": "bob", ...}

# Alice retrieves "main" (gets her version)
GET /api/v1/workflows/main
Headers: X-User-ID: alice

# Response
{"tag": "main", "owner": "alice", "artifact_id": "P5", ...}
```

## Architecture

### Components

```
┌─────────────────────┐
│  API Handler        │  ← Extracts X-User-ID header
│  (workflow.go)      │  ← Calls TagService.CreateTagWithNamespace()
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐
│  TagService         │  ← Validates tag name
│  (tag.go)           │  ← Calls BuildInternalTagName(username, userTag)
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐
│  tag_namespace.go   │  ← Builds: "alice/main"
│  (helpers)          │  ← Extracts: "main" from "alice/main"
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐
│  TagRepository      │  ← Stores "alice/main" in DB
│  (tag.go)           │  ← Queries WHERE tag_name = 'alice/main'
└─────────────────────┘
```

### Helper Functions

**File: `cmd/orchestrator/service/tag_namespace.go`**

| Function | Purpose | Example |
|----------|---------|---------|
| `BuildInternalTagName(user, tag)` | Add prefix | `("alice", "main")` → `"alice/main"` |
| `ExtractUserTagName(internal)` | Remove prefix | `"alice/main"` → `"main"` |
| `ExtractUsername(internal)` | Get owner | `"alice/main"` → `"alice"` |
| `ListUserTagPrefix(user)` | Filter prefix | `"alice"` → `"alice/"` |
| `IsGlobalTag(internal)` | Check if global | `"_global_/prod"` → `true` |
| `IsUserTag(internal, user)` | Check ownership | `("alice/main", "alice")` → `true"` |
| `CanAccessTag(internal, user)` | Check access | `("bob/main", "alice")` → `false` |
| `ValidateUserTagName(userTag)` | Validate input | `"main"` → `""` (valid) |

## API Changes

### Required Header

All tag operations now require the `X-User-ID` header:

```bash
curl -H "X-User-ID: alice" http://localhost:8081/api/v1/workflows/main
```

### Endpoint Changes

**No breaking changes** - endpoints remain the same:
- `POST /api/v1/workflows` - Create workflow with tag
- `GET /api/v1/workflows/{tag}` - Get workflow by tag
- `DELETE /api/v1/workflows/{tag}` - Delete workflow tag
- `GET /api/v1/tags` - List tags

### Response Format

Responses now include owner information:

```json
{
  "tag": "main",
  "owner": "alice",
  "target_id": "artifact-uuid",
  "target_kind": "patch_set"
}
```

**Global tags:**
```json
{
  "tag": "prod",
  "owner": "",  // Empty for global tags
  "target_id": "artifact-uuid",
  "target_kind": "dag_version"
}
```

### List Tags

Filter tags by scope:

```bash
# List user's tags only (default)
GET /api/v1/tags
Headers: X-User-ID: alice
Response: [{"tag": "main", "owner": "alice"}, {"tag": "feature-1", "owner": "alice"}]

# List global tags
GET /api/v1/tags?global=true
Response: [{"tag": "prod", "owner": ""}, {"tag": "staging", "owner": ""}]

# List all accessible tags (user's + global)
GET /api/v1/tags?all=true
Headers: X-User-ID: alice
Response: [{"tag": "main", "owner": "alice"}, {"tag": "prod", "owner": ""}]
```

## Code Examples

### Creating a Tag (API Handler)

```go
// cmd/orchestrator/handlers/workflow.go

func (h *WorkflowHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
    // Extract username from header
    username := r.Header.Get("X-User-ID")
    if username == "" {
        http.Error(w, "X-User-ID header required", http.StatusBadRequest)
        return
    }

    var req struct {
        TagName  string `json:"tag_name"`  // User provides: "main"
        Workflow json.RawMessage `json:"workflow"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    // ... create artifact ...

    // Create tag with automatic namespacing
    err := h.tagService.CreateTagWithNamespace(
        ctx,
        req.TagName,        // "main"
        username,           // "alice"
        models.KindPatchSet,
        artifactID,
        versionHash,
    )
    // Internally stored as: "alice/main"

    // Return clean response
    response := map[string]interface{}{
        "tag":    req.TagName,  // Return: "main" (not "alice/main")
        "owner":  username,     // Show ownership
        "artifact_id": artifactID,
    }
    json.NewEncoder(w).Encode(response)
}
```

### Getting a Tag (API Handler)

```go
func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
    username := r.Header.Get("X-User-ID")
    userTag := chi.URLParam(r, "tag")  // User requests: "main"

    // Lookup with automatic namespacing
    tag, err := h.tagService.GetTagWithNamespace(ctx, userTag, username)
    // Internally looks for: "alice/main"

    if err != nil {
        http.Error(w, "Tag not found", http.StatusNotFound)
        return
    }

    // Return clean response
    response := map[string]interface{}{
        "tag":   ExtractUserTagName(tag.TagName),  // "main"
        "owner": ExtractUsername(tag.TagName),     // "alice"
        // ... rest of data
    }
    json.NewEncoder(w).Encode(response)
}
```

### Listing Tags (API Handler)

```go
func (h *TagHandler) ListTags(w http.ResponseWriter, r *http.Request) {
    username := r.Header.Get("X-User-ID")

    var tags []*models.Tag
    var err error

    // Check query parameters
    if r.URL.Query().Get("global") == "true" {
        tags, err = h.tagService.ListGlobalTags(ctx)
    } else if r.URL.Query().Get("all") == "true" {
        tags, err = h.tagService.ListAllAccessibleTags(ctx, username)
    } else {
        tags, err = h.tagService.ListUserTags(ctx, username)
    }

    // Convert to response format (strip prefixes)
    response := make([]map[string]interface{}, len(tags))
    for i, tag := range tags {
        response[i] = map[string]interface{}{
            "tag":       ExtractUserTagName(tag.TagName),
            "owner":     ExtractUsername(tag.TagName),
            "target_id": tag.TargetID,
        }
    }

    json.NewEncoder(w).Encode(response)
}
```

## Migration

### For Existing Deployments

Run the migration script to prefix existing tags:

```bash
# Run migration
psql -U $DB_USER -d orchestrator -f migrations/003_add_tag_namespacing.sql

# Verify results
psql -U $DB_USER -d orchestrator -c "
SELECT
    CASE
        WHEN tag_name LIKE '_global_/%' THEN 'Global'
        WHEN tag_name LIKE '%/%' THEN 'User'
        ELSE 'Legacy'
    END as type,
    COUNT(*)
FROM tag
GROUP BY 1;
"
```

**What the migration does:**
1. Creates backup tables (`tag_backup_20251012`, `tag_move_backup_20251012`)
2. Prefixes tags with owner: `"main"` → `"alice/main"`
3. Marks ownerless tags as global: `"prod"` → `"_global_/prod"`
4. Updates `tag_move` table to match new names
5. Provides verification queries

**Migration examples:**
```sql
-- Before migration
tag_name: "main", moved_by: "alice"
tag_name: "feature", moved_by: "bob"
tag_name: "prod", moved_by: NULL

-- After migration
tag_name: "alice/main", moved_by: "alice"
tag_name: "bob/feature", moved_by: "bob"
tag_name: "_global_/prod", moved_by: NULL
```

### Rollback (if needed)

```sql
BEGIN;

-- Restore from backups
DROP TABLE tag;
ALTER TABLE tag_backup_20251012 RENAME TO tag;

DROP TABLE tag_move;
ALTER TABLE tag_move_backup_20251012 RENAME TO tag_move;

COMMIT;
```

## Global Tags

Global tags are system-wide and accessible to all users.

### Creating Global Tags

Use empty string for username:

```go
// Create global tag
err := tagService.CreateTagWithNamespace(ctx, "prod", "", kindDAGVersion, artifactID, hash)
// Stored as: "_global_/prod"
```

### Access Control

- **Read**: All users can read global tags
- **Write**: Only admins can create/modify/delete global tags
- **Enforce at API layer**: Check permissions before calling TagService

```go
// Example: Only admins can create global tags
if isGlobalTag && !isAdmin(username) {
    return fmt.Errorf("permission denied: only admins can create global tags")
}
```

## Query Patterns

### Find User's Tags

```sql
-- Get all tags for alice
SELECT * FROM tag
WHERE tag_name LIKE 'alice/%';

-- Get specific tag for alice
SELECT * FROM tag
WHERE tag_name = 'alice/main';
```

### Find Global Tags

```sql
-- Get all global tags
SELECT * FROM tag
WHERE tag_name LIKE '_global_/%';

-- Get specific global tag
SELECT * FROM tag
WHERE tag_name = '_global_/prod';
```

### Find All Accessible Tags

```sql
-- Get tags alice can access (her tags + global)
SELECT * FROM tag
WHERE tag_name LIKE 'alice/%'
   OR tag_name LIKE '_global_/%';
```

## Performance Considerations

### Index Performance

Tags with prefixes still use B-tree index efficiently:

```sql
-- Fast: Uses index with prefix
EXPLAIN SELECT * FROM tag WHERE tag_name = 'alice/main';
-- Index Scan using tag_pkey (cost=0.15..8.17 rows=1)

-- Fast: Uses index with LIKE prefix
EXPLAIN SELECT * FROM tag WHERE tag_name LIKE 'alice/%';
-- Index Scan using tag_pkey (cost=0.15..12.34 rows=10)
```

### Storage Impact

**Minimal storage increase:**
- Average username: 10 characters
- Separator: 1 character
- Total overhead: ~11 bytes per tag

**Example:**
- Before: `"main"` = 4 bytes
- After: `"alice/main"` = 10 bytes
- Increase: 6 bytes (~150%)

**At scale:**
- 10,000 tags × 11 bytes = 110 KB overhead (negligible)

## Security Considerations

### Validation

User-provided tag names are validated:

```go
// Validates:
// - Not empty
// - No "/" character
// - No "_global_" prefix
// - Length <= 100 characters

if err := ValidateUserTagName(userTag); err != "" {
    return fmt.Errorf("invalid tag: %s", err)
}
```

### Access Control

**Ownership checks:**
- Users can only modify their own tags
- Users can read their own tags + global tags
- Admins can modify global tags

**Enforcement:**
```go
// Before deleting
if !IsUserTag(tag.TagName, username) {
    return fmt.Errorf("access denied: cannot delete another user's tag")
}
```

### Injection Protection

Username is from authenticated header (not user input):

```go
// Safe: Username from auth middleware, not request body
username := r.Header.Get("X-User-ID")  // Set by auth layer

// NOT from request body (would be unsafe)
// username := req.Username  ← NEVER DO THIS
```

## Testing

### Test Multi-User Scenarios

```bash
# Test script: test_namespacing.sh

# Alice creates main
curl -X POST /api/v1/workflows \
  -H "X-User-ID: alice" \
  -d '{"tag_name": "main", ...}'

# Bob creates main (should succeed)
curl -X POST /api/v1/workflows \
  -H "X-User-ID: bob" \
  -d '{"tag_name": "main", ...}'

# Alice gets main (should get her version)
curl -H "X-User-ID: alice" /api/v1/workflows/main
# Returns: {"tag": "main", "owner": "alice", ...}

# Bob gets main (should get his version)
curl -H "X-User-ID: bob" /api/v1/workflows/main
# Returns: {"tag": "main", "owner": "bob", ...}
```

### Test Global Tags

```bash
# Admin creates global tag
curl -X POST /api/v1/workflows \
  -H "X-User-ID: admin" \
  -H "X-Admin: true" \
  -d '{"tag_name": "prod", "global": true, ...}'

# All users can read it
curl -H "X-User-ID: alice" /api/v1/workflows/prod
# Returns: {"tag": "prod", "owner": "", ...}
```

## Troubleshooting

### Issue: "Tag not found" after migration

**Cause:** API handler not using namespaced lookup

**Fix:** Update handler to use `GetTagWithNamespace()`:

```go
// ❌ Wrong
tag, err := tagService.GetTag(ctx, userTag)

// ✅ Correct
tag, err := tagService.GetTagWithNamespace(ctx, userTag, username)
```

### Issue: Users see "alice/main" instead of "main"

**Cause:** Response not stripping prefix

**Fix:** Use `ExtractUserTagName()` in response:

```go
// ❌ Wrong
response := {"tag": tag.TagName}  // Shows "alice/main"

// ✅ Correct
response := {"tag": ExtractUserTagName(tag.TagName)}  // Shows "main"
```

### Issue: Permission denied errors

**Cause:** User trying to access another user's tag

**Expected behavior:** Users can only access their own tags + global tags

**Verify ownership:**
```sql
-- Check tag ownership
SELECT tag_name, moved_by FROM tag WHERE tag_name = 'alice/main';
```

## FAQ

**Q: Can users have tags without prefixes?**
A: Legacy tags without prefixes are supported for backwards compatibility, but new tags will always have prefixes.

**Q: What happens to existing tags?**
A: Migration script automatically prefixes them with owner's username or marks them as global.

**Q: Can users see other users' tags?**
A: No, unless they are global tags. Users only see their own tags + global tags.

**Q: How do I create a shared tag?**
A: Create a global tag (requires admin permissions). It will be accessible to all users.

**Q: Can I change tag ownership?**
A: No. Tags are permanently owned by the creating user. To "transfer" a tag, create a new one and delete the old one.

**Q: What if username contains "/"?**
A: The auth layer should validate usernames to exclude special characters. If needed, URL-encode usernames.

## References

- Schema design: `docs/schema/DESIGN.md`
- Tag table: `docs/schema/TABLE_RELATIONSHIPS.md`
- Migration script: `migrations/003_add_tag_namespacing.sql`
- Implementation: `cmd/orchestrator/service/tag_namespace.go`

---

**Date:** 2025-10-12
**Status:** Implemented ✅
**Breaking Changes:** Requires X-User-ID header in all requests
