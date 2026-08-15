CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    password_hash text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_normalized CHECK (email = lower(btrim(email)) AND email <> ''),
    CONSTRAINT users_password_hash_present CHECK (password_hash <> '')
);
CREATE UNIQUE INDEX users_email_unique_idx ON users (email);

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name = btrim(name) AND name <> ''),
    slug text NOT NULL CHECK (slug = lower(btrim(slug)) AND slug <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX organizations_slug_unique_idx ON organizations (slug);

CREATE TABLE organization_memberships (
    organization_id uuid NOT NULL REFERENCES organizations (id),
    user_id uuid NOT NULL REFERENCES users (id),
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'editor', 'viewer')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX organization_memberships_user_idx ON organization_memberships (user_id, organization_id);

CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    name text NOT NULL CHECK (name = btrim(name) AND name <> ''),
    public_token text,
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    expires_at timestamptz,
    downloads_enabled boolean NOT NULL DEFAULT false,
    match_threshold double precision CHECK (match_threshold IS NULL OR match_threshold BETWEEN -1 AND 1),
    created_by_user_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    CONSTRAINT events_public_token_present CHECK (public_token IS NULL OR public_token = btrim(public_token) AND public_token <> '')
);
CREATE UNIQUE INDEX events_public_token_unique_idx ON events (public_token) WHERE public_token IS NOT NULL;
CREATE INDEX events_organization_created_idx ON events (organization_id, created_at DESC, id);
CREATE INDEX events_organization_status_idx ON events (organization_id, status, created_at DESC);

CREATE TABLE photos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    object_key text NOT NULL CHECK (object_key = btrim(object_key) AND object_key <> ''),
    original_filename text,
    content_type text,
    byte_size bigint CHECK (byte_size IS NULL OR byte_size >= 0),
    checksum_sha256 text CHECK (checksum_sha256 IS NULL OR checksum_sha256 ~ '^[0-9a-f]{64}$'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'uploading', 'uploaded', 'queued', 'processing', 'ready', 'failed', 'deleted')),
    failure_code text,
    created_by_user_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, object_key),
    FOREIGN KEY (organization_id, event_id) REFERENCES events (organization_id, id)
);
CREATE INDEX photos_event_created_idx ON photos (organization_id, event_id, created_at DESC, id);
CREATE INDEX photos_event_status_idx ON photos (organization_id, event_id, status);
CREATE INDEX photos_pending_work_idx ON photos (status, updated_at) WHERE status IN ('queued', 'processing');

CREATE TABLE faces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    photo_id uuid NOT NULL,
    face_index integer NOT NULL CHECK (face_index >= 0),
    vector_point_id uuid NOT NULL UNIQUE,
    bounding_box jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, photo_id, face_index),
    FOREIGN KEY (organization_id, event_id) REFERENCES events (organization_id, id),
    FOREIGN KEY (organization_id, photo_id) REFERENCES photos (organization_id, id)
);
CREATE INDEX faces_event_photo_idx ON faces (organization_id, event_id, photo_id);

CREATE TABLE outbox_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    aggregate_type text NOT NULL CHECK (aggregate_type = btrim(aggregate_type) AND aggregate_type <> ''),
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type = btrim(event_type) AND event_type <> ''),
    payload jsonb NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key <> ''),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'publishing', 'published', 'failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, idempotency_key)
);
CREATE INDEX outbox_publish_idx ON outbox_messages (available_at, created_at) WHERE status IN ('pending', 'failed');
CREATE INDEX outbox_organization_created_idx ON outbox_messages (organization_id, created_at DESC);

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    refresh_token_hash text NOT NULL UNIQUE CHECK (refresh_token_hash <> ''),
    family_id uuid NOT NULL,
    replaced_by_session_id uuid REFERENCES auth_sessions (id),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'rotated', 'revoked', 'expired')),
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT auth_sessions_expiry CHECK (expires_at > created_at)
);
CREATE INDEX auth_sessions_user_status_idx ON auth_sessions (user_id, status, expires_at);
CREATE INDEX auth_sessions_family_idx ON auth_sessions (family_id, created_at);

CREATE TABLE searches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('started', 'completed', 'rejected', 'failed')),
    consent_recorded boolean NOT NULL CHECK (consent_recorded),
    result_count integer NOT NULL DEFAULT 0 CHECK (result_count >= 0),
    failure_code text,
    requested_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, event_id) REFERENCES events (organization_id, id)
);
CREATE INDEX searches_event_requested_idx ON searches (organization_id, event_id, requested_at DESC, id);

CREATE TABLE download_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    photo_id uuid,
    search_id uuid,
    requested_by_user_id uuid REFERENCES users (id),
    kind text NOT NULL CHECK (kind IN ('single', 'bulk')),
    decision text NOT NULL CHECK (decision IN ('allowed', 'denied')),
    denial_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, event_id) REFERENCES events (organization_id, id),
    FOREIGN KEY (organization_id, photo_id) REFERENCES photos (organization_id, id),
    FOREIGN KEY (organization_id, search_id) REFERENCES searches (organization_id, id),
    CONSTRAINT download_single_photo CHECK (kind <> 'single' OR photo_id IS NOT NULL)
);
CREATE INDEX download_records_event_created_idx ON download_records (organization_id, event_id, created_at DESC);
CREATE INDEX download_records_user_created_idx ON download_records (organization_id, requested_by_user_id, created_at DESC) WHERE requested_by_user_id IS NOT NULL;

CREATE TABLE audit_records (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid REFERENCES organizations (id),
    actor_user_id uuid REFERENCES users (id),
    action text NOT NULL CHECK (action = btrim(action) AND action <> ''),
    resource_type text NOT NULL CHECK (resource_type = btrim(resource_type) AND resource_type <> ''),
    resource_id uuid,
    outcome text NOT NULL CHECK (outcome IN ('success', 'denied', 'failure')),
    request_id text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_records_organization_created_idx ON audit_records (organization_id, created_at DESC, id DESC);
CREATE INDEX audit_records_resource_idx ON audit_records (resource_type, resource_id, created_at DESC) WHERE resource_id IS NOT NULL;
