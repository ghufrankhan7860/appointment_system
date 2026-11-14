ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_booked_via_check;

ALTER TABLE appointments
DROP COLUMN IF EXISTS booked_via;
