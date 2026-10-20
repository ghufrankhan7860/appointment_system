CREATE TABLE specialities (
    id BIGSERIAL PRIMARY KEY,

    slug VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(150) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE doctor_specialities (
    doctor_id BIGINT NOT NULL,
    speciality_id BIGINT NOT NULL,

    is_primary BOOLEAN NOT NULL DEFAULT FALSE,

    PRIMARY KEY (doctor_id, speciality_id),

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE,
    FOREIGN KEY (speciality_id) REFERENCES specialities(id)
);

CREATE INDEX idx_doctor_specialities_speciality_id
    ON doctor_specialities (speciality_id);

INSERT INTO specialities (slug, name, created_at) VALUES
    ('general-medicine', 'General Medicine', NOW()),
    ('cardiology', 'Cardiology', NOW()),
    ('dermatology', 'Dermatology', NOW()),
    ('paediatrics', 'Paediatrics', NOW()),
    ('gynaecology', 'Gynaecology', NOW()),
    ('orthopaedics', 'Orthopaedics', NOW()),
    ('ent', 'ENT', NOW()),
    ('psychiatry', 'Psychiatry', NOW()),
    ('diabetology', 'Diabetology', NOW()),
    ('neurology', 'Neurology', NOW()),
    ('gastroenterology', 'Gastroenterology', NOW()),
    ('ophthalmology', 'Ophthalmology', NOW()),
    ('dentistry', 'Dentistry', NOW()),
    ('pulmonology', 'Pulmonology', NOW()),
    ('endocrinology', 'Endocrinology', NOW());