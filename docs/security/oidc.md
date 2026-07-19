# OAuth2 / OIDC / JWT Authentication

MagiC supports enterprise SSO via OpenID Connect. When enabled, the
gateway accepts JWT bearer tokens issued by your identity provider
alongside the existing `MAGIC_API_KEY`. Worker tokens (`mct_` prefix)
are unaffected.

Verified against any OIDC-compliant provider, including:

- **Okta** — `https://<tenant>.okta.com`
- **Azure AD / Microsoft Entra** — `https://login.microsoftonline.com/<tenant-id>/v2.0`
- **Auth0** — `https://<tenant>.auth0.com/`
- **Google Workspace** — `https://accounts.google.com`
- **Keycloak** — `https://<host>/realms/<realm>`

## Configuration

| Env var | Required | Description |
|---|---|---|
| `MAGIC_OIDC_ISSUER` | yes | Issuer URL. MagiC discovers `<issuer>/.well-known/openid-configuration` at startup. |
| `MAGIC_OIDC_CLIENT_ID` | yes* | Expected `aud` claim. *Required unless `MAGIC_OIDC_AUDIENCE` is set. |
| `MAGIC_OIDC_AUDIENCE` | optional | Override `aud` (use when access-token `aud` differs from client ID — common on Auth0 / Okta custom authz servers). |

If `MAGIC_OIDC_ISSUER` is unset, OIDC is disabled and behavior is
identical to previous releases (API-key auth only).

## How it works

1. Client sends `Authorization: Bearer <jwt>`.
2. Gateway middleware inspects the token. If shaped like a JWT
   (`ey...` with 3 dot-separated segments), it verifies against the
   issuer's JWKS (signature, `iss`, `aud`, `exp`, `nbf`).
3. On success, claims (`sub`, `email`, `org_id`, `roles`) are attached
   to the request context, and the API-key check is bypassed.
4. On failure, 401 is returned. Non-JWT tokens (opaque API keys, worker
   tokens) fall through to their respective middlewares — fully
   backward compatible.

JWKS keys are cached and auto-refreshed; a 60-second clock-skew
tolerance is applied.

## Custom claims mapping

MagiC reads two non-standard claims for authorization:

- `org_id` — the org the user belongs to. Used by the RBAC middleware
  to scope the request.
- `roles` — array of role names (`owner`, `admin`, `viewer`). If
  present, bypasses the store-backed role-binding check.

Your IdP must be configured to include these in the token. Typical
mappings:

- **Okta** — Authorization Server → Claims → add `org_id` (from user
  profile attribute) and `roles` (from group memberships).
- **Azure AD** — App registration → Token configuration → add optional
  claim from group or extension attribute.
- **Auth0** — Action on Login flow: `api.idToken.setCustomClaim("org_id", event.user.app_metadata.org_id)`.
- **Google Workspace** — only standard claims; use path-scoped RBAC
  (`/orgs/{orgID}/...`) or map via an intermediary IdP.

## Per-provider setup

### Okta

1. Admin → Applications → Create App Integration → OIDC / Web.
2. Copy **Client ID**.
3. Issuer = `https://<tenant>.okta.com` (default authorization server)
   or `https://<tenant>.okta.com/oauth2/<custom>` (custom authz server).
4. Set `MAGIC_OIDC_ISSUER`, `MAGIC_OIDC_CLIENT_ID`.

### Azure AD (Entra)

1. Entra → App registrations → New → Web.
2. Copy **Application (client) ID** and **Tenant ID**.
3. Issuer = `https://login.microsoftonline.com/<tenant-id>/v2.0`.
4. For access tokens (not ID tokens), set `MAGIC_OIDC_AUDIENCE` to the
   API's Application ID URI (e.g. `api://magic`).

### Auth0

1. Dashboard → Applications → Create Application → Regular Web App.
2. Copy **Client ID**.
3. Issuer = `https://<tenant>.auth0.com/` (trailing slash required).
4. If using API authorization, set `MAGIC_OIDC_AUDIENCE` to your
   API identifier.

### Google Workspace

1. Cloud Console → APIs & Services → Credentials → OAuth client ID →
   Web application.
2. Issuer = `https://accounts.google.com`.
3. Client ID → `MAGIC_OIDC_CLIENT_ID`.

### Keycloak

1. Create realm and client (Access Type: confidential or public).
2. Issuer = `https://<host>/realms/<realm>`.
3. Map realm roles to a `roles` token claim.

## Gotchas

- **Clock skew** — 60s tolerance baked in. Beyond that, ensure NTP on
  the MagiC host.
- **Key rotation** — JWKS is fetched lazily and cached by go-oidc;
  rotation is seamless.
- **Audience mismatch** — most common cause of 401. Verify `aud` in
  the decoded token (jwt.io) matches `MAGIC_OIDC_CLIENT_ID` or
  `MAGIC_OIDC_AUDIENCE`.
- **Discovery timeout** — 10s at startup. If the provider is slow or
  unreachable, the server fails to start (fail-fast by design).
- **Token size** — large group/role claims can blow up cookie size in
  browser flows; prefer scoped roles in the JWT.

## Not yet supported

- Token introspection (RFC 7662) — opaque access tokens.
- Refresh-token flow orchestrated by MagiC (clients handle this today).
- Device authorization grant (RFC 8628).
- mTLS client authentication.
