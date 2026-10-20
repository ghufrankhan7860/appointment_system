CREATE TABLE doctors (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE,

    full_name VARCHAR(150) NOT NULL,
    registration_number VARCHAR(64) NOT NULL,
    registration_council VARCHAR(150) NOT NULL,
    registration_year SMALLINT NULL,
    qualification VARCHAR(255) NOT NULL,
    years_of_experience SMALLINT NOT NULL DEFAULT 0 CHECK (years_of_experience >= 0),
    bio TEXT NOT NULL DEFAULT '',

    consultation_fee INTEGER NOT NULL DEFAULT 0 CHECK (consultation_fee >= 0),
    slot_duration_min SMALLINT NOT NULL DEFAULT 15 CHECK (slot_duration_min > 0),

    clinic_name VARCHAR(150) NOT NULL DEFAULT '',
    clinic_address VARCHAR(500) NOT NULL DEFAULT '',
    city VARCHAR(100) NOT NULL DEFAULT '',
    pincode VARCHAR(10) NOT NULL DEFAULT '',

    verification_status VARCHAR(30) NOT NULL DEFAULT 'pending' CHECK (verification_status IN ('pending', 'verified', 'rejected', 'suspended')),
    verified_at TIMESTAMPTZ NULL,
    is_accepting_patients BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    UNIQUE (registration_council, registration_number),

    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_doctors_city_verification_status
    ON doctors (LOWER(city), verification_status);