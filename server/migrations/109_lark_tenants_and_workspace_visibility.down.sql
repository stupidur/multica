DROP INDEX IF EXISTS idx_workspace_visibility_home_tenant;
DROP INDEX IF EXISTS idx_workspace_home_tenant_id;

ALTER TABLE workspace
    DROP COLUMN IF EXISTS visibility,
    DROP COLUMN IF EXISTS home_tenant_id;

DROP TABLE IF EXISTS lark_tenant_admin;

DROP INDEX IF EXISTS idx_user_identity_provider_email;
DROP INDEX IF EXISTS idx_user_identity_user_id;
DROP TABLE IF EXISTS user_identity;

DROP TABLE IF EXISTS lark_tenant;
