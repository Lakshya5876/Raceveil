# corpus/ — seeded vulnerable corpus + synchronized-safe twins

Design/TEST_PLAN.md Layer 5/6 evidence: purpose-built vulnerable
applications across two independent stacks (Node/Express, Spring Boot),
each race archetype paired with a synchronized-safe twin that RaceVeil must
stay clean on. Everything here is local-only, free, and never deployed or
billed anywhere (Context/DECISIONS.md ADR-010/ADR-011).

## Bring it up

```bash
# 1. Postgres (shared by both stacks, 127.0.0.1-only)
cd corpus
docker compose up -d
# wait for it to report healthy:
docker compose ps

# 2. Node/Express corpus (port 4000)
cd node
npm install
npm start
# "raceveil corpus (node) listening on 127.0.0.1:4000"

# 3. Spring Boot corpus (port 4100), in another terminal
cd ../spring-boot
mvn spring-boot:run
```

Both apps read their DB connection from hardcoded local config (see
`node/server.js`, `spring-boot/src/main/resources/application.properties`)
pointed at the `docker compose` Postgres — no environment variables or
secrets to set up.

## Race archetypes

| Archetype | Vulnerable endpoint | Safe twin | Invariant |
|---|---|---|---|
| Coupon double-redemption | `POST /coupon/redeem` | `POST /coupon/redeem-safe` | `max_successes=1` |
| Gift-card double-spend | `POST /giftcard/redeem` | `POST /giftcard/redeem-safe` | `max_successes=1` |
| Inventory overrun | `POST /inventory/reserve` | `POST /inventory/reserve-safe` | `max_successes=1` |
| Duplicate order confirmation | `POST /order/confirm` | `POST /order/confirm-safe` | `max_successes=1` |
| Duplicate signup | `POST /signup` | `POST /signup-safe` | `uniqueness=1` |
| Login-attempt limit bypass | `POST /login` | `POST /login-safe` | `monotonic_limit` |

The vulnerable twin uses a check-then-act pattern (a `SELECT` followed by an
`UPDATE`/`INSERT` with no lock or transaction isolation strong enough to
prevent the race); the safe twin does the equivalent work in one atomic
statement or an explicit row-locking transaction. Node has all six
archetypes; Spring Boot has coupon/inventory/order as a second-stack check
that RaceVeil isn't overfit to one framework's response idioms.

## Run RaceVeil against it

```bash
raceveil scan --candidate node/candidates/coupon-vuln.yaml \
  --scope node/candidates/scope.yaml \
  --auth node/candidates/session.json \
  --out run --no-safe-mode --i-am-authorized
```

Or point discovery at the whole app (no hand-written candidate):

```bash
raceveil scan http://127.0.0.1:4000 \
  --scope node/candidates/scope.yaml \
  --auth node/candidates/session.json \
  --out run --no-safe-mode --i-am-authorized --max-candidates 20
```

`results/` and `results-spring/` hold saved `Oracle`+`Finding` JSON from
past runs (one per archetype, vulnerable and safe) as a checked-in evidence
snapshot — see the root [`README.md`](../README.md)'s evidence table for
the summary.
