CREATE UNIQUE INDEX photos_organization_event_id_idx
    ON photos (organization_id, event_id, id);

CREATE TABLE photo_upload_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    photo_id uuid NOT NULL,
    upload_id text NOT NULL,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'aborted', 'expired')),
    expires_at timestamptz NOT NULL,
    completed_at timestamptz,
    aborted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, upload_id),
    FOREIGN KEY (organization_id, event_id, photo_id)
        REFERENCES photos (organization_id, event_id, id),
    CHECK (expires_at > created_at),
    CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CHECK ((status = 'aborted') = (aborted_at IS NOT NULL))
);

CREATE UNIQUE INDEX photo_upload_sessions_active_photo_idx
    ON photo_upload_sessions (organization_id, event_id, photo_id)
    WHERE status = 'active';

CREATE INDEX photo_upload_sessions_expiry_idx
    ON photo_upload_sessions (expires_at)
    WHERE status = 'active';
