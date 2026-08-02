# Migrations

Add ordered schema migrations here only when a database schema is introduced.

For each change:

1. Use a sortable version prefix, for example `000001_create_appointments.up.sql`.
2. Add the matching rollback when rollback is supported, or document a forward fix.
3. Keep migrations compatible with the deployment rollout order.
4. Put large, resumable data changes in `scripts/backfill`, not in startup migration code.
5. Test clean installation and the supported upgrade path against the target database.
