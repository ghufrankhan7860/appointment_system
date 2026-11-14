ALTER TABLE appointments
ADD COLUMN booked_via VARCHAR(10) NOT NULL DEFAULT 'app';

ALTER TABLE appointments
ADD CONSTRAINT appointments_booked_via_check CHECK (booked_via IN ('app', 'desk'));
