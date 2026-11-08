# Zone ↔ egg mapping

Edges require evidence classes:

- `explicit_public_field`
- `derived_from_explicit_public_fields`
- `user_observed`
- `name_heuristic_unverified`
- `unresolved`

Name-only joins stay heuristics and are never promoted to authoritative.

Secret Room Egg is not assumed to be an ordinary permanent-zone egg. Missing Zones / Worlds / SecretRooms collections must not crash sync — relationships remain unresolved.
