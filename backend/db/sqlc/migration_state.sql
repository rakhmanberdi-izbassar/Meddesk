-- golang-migrate owns this table. This declaration is for sqlc analysis only;
-- application migrations must not create or mutate migration bookkeeping.
CREATE TABLE schema_migrations (
    version bigint NOT NULL PRIMARY KEY,
    dirty boolean NOT NULL
);
