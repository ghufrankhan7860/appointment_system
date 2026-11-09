CREATE UNIQUE INDEX idx_user_patient_patient_id_self_unique
    ON user_patient (patient_id)
    WHERE patient_relationship = 'self' AND status = 'active';
