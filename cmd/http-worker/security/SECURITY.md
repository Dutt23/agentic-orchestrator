# HTTP Worker Security Architecture

This package implements defense-in-depth security for HTTP requests to prevent SSRF (Server-Side Request Forgery) and other attacks.

## Architecture

The security validation is split into focused, testable modules:

```
URLValidator (orchestrator)
├── ProtocolValidator (protocol scheme validation)
├── HostValidator (hostname + DNS validation)
│   └── IPValidator (IP address validation)
└── PathValidator (path + query parameter validation)
```

## Validators

### 1. ProtocolValidator

**Purpose**: Ensure only safe protocols are used

**Allows**:
- `http://`
- `https://`

**Blocks**:
- `file://` - Local file system access
- `jdbc://`, `mysql://`, `postgres://` - Database connections
- `ftp://`, `ssh://`, `telnet://` - Other protocols
- `dict://`, `gopher://` - SSRF attack vectors
- `redis://`, `mongodb://` - Database protocols

### 2. HostValidator

**Purpose**: Prevent access to internal/private networks

**Blocks by hostname**:
- `localhost`
- `127.0.0.1`, `::1`
- `0.0.0.0`, `::`

**Delegates to IPValidator** for DNS-resolved IPs.

### 3. IPValidator

**Purpose**: Validate IP addresses after DNS resolution

**Blocks**:
- **Loopback**: `127.0.0.0/8`, `::1`
- **Private networks**: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`
- **Link-local**: `169.254.0.0/16` (AWS metadata: `169.254.169.254`), `fe80::/10`
- **Multicast**: `224.0.0.0/4`, `ff00::/8`
- **Unspecified**: `0.0.0.0`, `::`

### 4. PathValidator

**Purpose**: Prevent file access and path traversal

**Blocks**:
- `../` - Path traversal
- `/etc/`, `/proc/`, `/sys/` - System directories
- `c:/`, `c:\` - Windows drives
- URL-encoded variants: `%2e%2e%2f`

## Attack Scenarios Prevented

### 1. SSRF to Internal Services
```
❌ Blocked: http://localhost:6379/
❌ Blocked: http://192.168.1.100/admin
✅ Allowed: https://api.example.com/data
```

### 2. Cloud Metadata Service Access
```
❌ Blocked: http://169.254.169.254/latest/meta-data/
❌ Blocked: http://metadata.google.internal/
✅ Allowed: https://api.aws.amazon.com/
```

### 3. Protocol Injection
```
❌ Blocked: file:///etc/passwd
❌ Blocked: jdbc:mysql://db.internal:3306/
❌ Blocked: redis://cache.internal:6379
✅ Allowed: https://api.example.com/
```

### 4. Path Traversal
```
❌ Blocked: https://example.com/../../../etc/passwd
❌ Blocked: https://example.com/api?file=../../secrets
✅ Allowed: https://example.com/api/data
```

### 5. DNS Rebinding
```
Attacker sets up: evil.com → 1.2.3.4 (public) initially
Then changes to: evil.com → 127.0.0.1 (localhost)

Protection: DNS lookup happens at validation time, IPs checked before request
```

## Usage

```go
// In worker initialization
worker := &HTTPWorker{
    urlValidator: security.NewURLValidator(),
    // ...
}

// Before making HTTP request
if err := worker.urlValidator.Validate(urlStr); err != nil {
    return fmt.Errorf("URL blocked for security: %w", err)
}
```

## Testing

Each validator can be unit tested independently:

```go
func TestProtocolValidator(t *testing.T) {
    validator := NewProtocolValidator()

    // Should pass
    assert.Nil(t, validator.Validate("https"))

    // Should fail
    assert.Error(t, validator.Validate("file"))
    assert.Error(t, validator.Validate("jdbc"))
}
```

## Security Best Practices

1. **Defense in Depth**: Multiple layers of validation
2. **Fail Secure**: Unknown/ambiguous cases are blocked
3. **Explicit Allow**: Only explicitly allowed patterns pass
4. **DNS Resolution**: Check IPs after DNS lookup (prevents DNS rebinding)
5. **Query Parameter Validation**: Check for injection in parameters

## Adding New Rules

**To block a new protocol:**
Edit `protocol_validator.go`, add to `GetBlockedProtocols()`

**To block a new hostname pattern:**
Edit `host_validator.go`, add to `blockedHostnames`

**To block a new path pattern:**
Edit `path_validator.go`, add to `blockedPatterns`

**To add custom IP range blocking:**
Edit `ip_validator.go`, add custom logic in `Validate()`

## References

- [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [AWS IMDS Security](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html)
- [CWE-918: SSRF](https://cwe.mitre.org/data/definitions/918.html)
