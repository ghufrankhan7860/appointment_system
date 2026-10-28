ALTER TABLE appointment_reschedule_proposals
DROP CONSTRAINT IF EXISTS appointment_reschedule_proposals_status_check;

ALTER TABLE appointment_reschedule_proposals
ADD CONSTRAINT appointment_reschedule_proposals_status_check CHECK (status IN ('pending', 'accepted', 'declined', 'expired', 'cancelled'));
