CREATE TABLE patients (
    id BIGSERIAL PRIMARY KEY,

    name VARCHAR(150) NOT NULL,
    date_of_birth DATE NOT NULL,

    gender VARCHAR(20) CHECK (gender IN ('Male', 'Female', 'Other', 'Unknown')),
    blood_type VARCHAR(3) CHECK (blood_type IN ('A+', 'A-', 'B+', 'B-', 'AB+', 'AB-', 'O+', 'O-')),
    emergency_contact VARCHAR(150),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);