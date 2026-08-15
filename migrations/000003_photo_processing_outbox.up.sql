ALTER TABLE photos
    ADD COLUMN processing_generation integer NOT NULL DEFAULT 0
        CHECK (processing_generation >= 0);
