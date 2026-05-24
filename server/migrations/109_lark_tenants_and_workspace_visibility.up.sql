CREATE TABLE lark_tenant (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_identity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    tenant_id UUID REFERENCES lark_tenant(id) ON DELETE CASCADE,
    external_user_id TEXT NOT NULL,
    union_id TEXT,
    email TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, tenant_id, external_user_id)
);

CREATE INDEX idx_user_identity_user_id ON user_identity(user_id);
CREATE INDEX idx_user_identity_provider_email ON user_identity(provider, email) WHERE email IS NOT NULL;

CREATE TABLE lark_tenant_admin (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES lark_tenant(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);

ALTER TABLE workspace
    ADD COLUMN home_tenant_id UUID REFERENCES lark_tenant(id) ON DELETE SET NULL,
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'tenant'));

CREATE INDEX idx_workspace_home_tenant_id ON workspace(home_tenant_id);
CREATE INDEX idx_workspace_visibility_home_tenant ON workspace(visibility, home_tenant_id);
