ALTER TABLE appointments
ADD COLUMN rescheduled_from_id BIGINT NULL;

ALTER TABLE appointments
ADD FOREIGN KEY (rescheduled_from_id) REFERENCES appointments(id);
