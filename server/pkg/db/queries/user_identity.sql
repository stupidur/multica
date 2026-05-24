-- name: GetUserIdentityByProviderTenantExternal :one
SELECT * FROM user_identity
WHERE provider = $1 AND tenant_id = $2 AND external_user_id = $3;

-- name: UpsertUserIdentity :one
INSERT INTO user_identity (user_id, provider, tenant_id, external_user_id, union_id, email)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (provider, tenant_id, external_user_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    union_id = EXCLUDED.union_id,
    email = EXCLUDED.email,
    updated_at = now()
RETURNING *;
