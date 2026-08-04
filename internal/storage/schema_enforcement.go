package storage

// schemaV8 identifies whether durable action authority came from a manual
// single-use approval or the narrow automatic enforcement lane. Historical
// action rows are manual by construction.
const schemaV8 = `
ALTER TABLE actions ADD COLUMN authority TEXT NOT NULL DEFAULT 'manual';
CREATE INDEX actions_authority_idx ON actions(authority, requested_ns);
`
