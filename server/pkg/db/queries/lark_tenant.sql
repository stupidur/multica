-- name: GetLarkTenant :one
SELECT * FROM lark_tenant
WHERE id = $1;

-- name: GetLarkTenantByKey :one
SELECT * FROM lark_tenant
WHERE tenant_key = $1;

-- name: UpsertLarkTenantByKey :one
INSERT INTO lark_tenant (tenant_key, name)
VALUES ($1, $2)
ON CONFLICT (tenant_key) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = now()
RETURNING *;

-- name: CountLarkTenantAdmins :one
SELECT count(*)::bigint FROM lark_tenant_admin
WHERE tenant_id = $1;

-- name: GetLarkTenantAdminByTenantAndUser :one
SELECT * FROM lark_tenant_admin
WHERE tenant_id = $1 AND user_id = $2;

-- name: UpsertLarkTenantAdmin :one
INSERT INTO lark_tenant_admin (tenant_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    updated_at = now()
RETURNING *;

-- name: UpdateLarkTenantSettings :one
UPDATE lark_tenant
SET settings = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
