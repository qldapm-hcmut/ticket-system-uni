# Database workspace

- `migrations/`: versioned `.up.sql` / `.down.sql` migrations (not implemented yet).
- `queries/`: SQL source for sqlc (not implemented yet).
- `seed/`: bootstrap units and the first Admin email (not implemented yet).

The schema design currently lives in `docs/db-schema-and-api-contract.md`.
No placeholder migrations or generated database code are created by the skeleton.
`make migrate-*`, `make seed`, and `make sqlc` are reserved for the database implementation; they require their CLI tools and SQL/config files first.
