CREATE TABLE doctor_tags (
    doctor_id BIGINT NOT NULL,

    tag VARCHAR(100) NOT NULL,
    tag_type VARCHAR(30) NOT NULL DEFAULT 'keyword' CHECK (tag_type IN ('keyword', 'condition', 'procedure', 'language')),

    PRIMARY KEY (doctor_id, tag),

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
);

CREATE INDEX idx_doctor_tags_tag
    ON doctor_tags (tag);