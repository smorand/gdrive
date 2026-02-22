# Authentication Documentation

## Two Authentication Modes

### CLI Mode (existing)
- User authenticates via local OAuth2 browser flow
- Token stored in `~/.gdrive/token.json`
- Single user per machine
- `auth.NewConfig()` → `GetAuthenticatedService()`

### MCP Mode (new)
- Per-request authentication via context-injected tokens
- OAuth2 authorization server proxies to Google OAuth
- Multi-user via Bearer tokens
- `auth.GetClientFromContext(ctx)` → `GetAuthenticatedServiceWithContext(ctx)`

## Context Injection (internal/auth/auth.go)

```go
// Store in context
ctx = auth.WithOAuthConfig(ctx, oauthConfig)
ctx = auth.WithAccessToken(ctx, token)

// Retrieve from context
config, ok := auth.GetOAuthConfigFromContext(ctx)
token, ok := auth.GetAccessTokenFromContext(ctx)

// Get authenticated HTTP client
client := auth.GetClientFromContext(ctx)

// Get Drive/Activity service
driveSrv, err := auth.GetAuthenticatedServiceWithContext(ctx, cfg)
activitySrv, err := auth.GetAuthenticatedActivityServiceWithContext(ctx, cfg)
```

## OAuth2 Server (internal/mcp/oauth2.go)

### Standards Compliance
- RFC 8414: OAuth Authorization Server Metadata
- RFC 9728: OAuth Protected Resource Metadata
- RFC 7591: Dynamic Client Registration
- PKCE S256 (required for all flows)

### Endpoints
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/.well-known/oauth-protected-resource` | GET | No | Resource metadata |
| `/.well-known/oauth-authorization-server` | GET | No | Server metadata |
| `/oauth/register` | POST | No | Dynamic client registration |
| `/oauth/authorize` | GET | No | Start authorization flow |
| `/oauth/callback` | GET | No | Google OAuth callback |
| `/oauth/token` | POST | No | Token exchange |

### In-Memory Stores (ephemeral, recreatable)
- **Registered Clients**: `clientId` → `{clientSecret, redirectURIs[], createdAt}`
- **Auth States**: `state` → `{clientId, redirectURI, codeChallenge, clientState, createdAt}` (10 min TTL)
- **Auth Codes**: `code` → `{clientId, redirectURI, codeChallenge, googleToken, createdAt}` (10 min TTL)

### Credential Loading Priority
1. GCP Secret Manager (`--secret-name` + `--secret-project`)
2. Local credential file (`--credential-file`)

Supports three JSON formats:
- Web format: `{"web": {"client_id": "...", "client_secret": "..."}}`
- Installed format: `{"installed": {"client_id": "...", "client_secret": "..."}}`
- Flat format: `{"client_id": "...", "client_secret": "..."}`

### Flow Sequence

```
1. Client → POST /oauth/register
   ← 201 {client_id, client_secret}

2. Client → GET /oauth/authorize?
     client_id=...&redirect_uri=...&state=...&
     code_challenge=BASE64URL(SHA256(verifier))&
     code_challenge_method=S256
   ← 302 → Google OAuth consent screen

3. User authenticates with Google
   Google → GET /oauth/callback?code=...&state=...
   Server exchanges Google code for tokens
   ← 302 → client redirect_uri?code=OUR_CODE&state=CLIENT_STATE

4. Client → POST /oauth/token
     grant_type=authorization_code&code=OUR_CODE&
     code_verifier=ORIGINAL_VERIFIER&client_id=...&client_secret=...
   Server validates PKCE: SHA256(verifier) == stored challenge
   ← 200 {access_token, token_type, expires_in}

5. Client → POST /mcp (Authorization: Bearer ACCESS_TOKEN)
   Server validates token, injects into context
   ← MCP response
```

### Auth Middleware (internal/mcp/server.go)

Two layers:
1. `authMiddleware`: HTTP-level Bearer token enforcement with WWW-Authenticate headers
2. `httpContextFunc`: Injects validated OAuth config + token into MCP context
