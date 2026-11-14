CREATE TABLE doctor_staff (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,

    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'removed')),
    removed_at TIMESTAMPTZ NULL,
    removed_by_user_id BIGINT NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (doctor_id) REFERENCES doctors(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (removed_by_user_id) REFERENCES users(id)
);

-- A staff account works for one doctor at a time.
CREATE UNIQUE INDEX idx_doctor_staff_user_id_active_unique
    ON doctor_staff (user_id)
    WHERE status = 'active';

CREATE INDEX idx_doctor_staff_doctor_id_status
    ON doctor_staff (doctor_id, status);

CREATE TABLE doctor_staff_invites (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL,
    created_by_user_id BIGINT NOT NULL,

    code_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'redeemed', 'revoked', 'expired')),

    expires_at TIMESTAMPTZ NOT NULL,
    redeemed_by_user_id BIGINT NULL,
    redeemed_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (doctor_id) REFERENCES doctors(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    FOREIGN KEY (redeemed_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_doctor_staff_invites_code_hash_unique
    ON doctor_staff_invites (code_hash);

CREATE INDEX idx_doctor_staff_invites_doctor_id_status
    ON doctor_staff_invites (doctor_id, status);

CREATE TABLE staff_invite_attempts (
    id BIGSERIAL PRIMARY KEY,
    client_ip VARCHAR(45) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_staff_invite_attempts_client_ip_created_at
    ON staff_invite_attempts (client_ip, created_at);
