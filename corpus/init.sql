-- RaceVeil Phase 2 corpus schema (Design/TEST_PLAN.md Layer 5).
-- Six race archetypes, each as a vuln/safe table pair so the two corpus
-- apps (Node/Express, Spring Boot) never share mutable state with each
-- other's runs. "vuln" tables are mutated by check-then-act application
-- code with no locking; "safe" tables are mutated by the identical
-- check-then-act logic wrapped in one atomic statement/transaction.

-- Archetype 1 (max_successes): check-then-act coupon redemption.
CREATE TABLE coupons_vuln (code TEXT PRIMARY KEY, used BOOLEAN NOT NULL DEFAULT FALSE);
CREATE TABLE coupons_safe (code TEXT PRIMARY KEY, used BOOLEAN NOT NULL DEFAULT FALSE);

-- Archetype 2 (uniqueness): duplicate resource creation on signup.
-- vuln has no unique constraint; safe does, so the DB itself enforces it.
CREATE TABLE signups_vuln (id SERIAL PRIMARY KEY, username TEXT NOT NULL);
CREATE TABLE signups_safe (id SERIAL PRIMARY KEY, username TEXT NOT NULL UNIQUE);

-- Archetype 3 (max_successes): duplicate redemption of a one-time gift
-- card, distinct resource/table from archetype 1, with a balance field for
-- Level 2 body-differential testing.
CREATE TABLE giftcards_vuln (code TEXT PRIMARY KEY, redeemed BOOLEAN NOT NULL DEFAULT FALSE, balance INT NOT NULL DEFAULT 50);
CREATE TABLE giftcards_safe (code TEXT PRIMARY KEY, redeemed BOOLEAN NOT NULL DEFAULT FALSE, balance INT NOT NULL DEFAULT 50);

-- Archetype 4 (monotonic_limit): inventory overrun on the last unit.
CREATE TABLE inventory_vuln (sku TEXT PRIMARY KEY, stock INT NOT NULL);
CREATE TABLE inventory_safe (sku TEXT PRIMARY KEY, stock INT NOT NULL);

-- Archetype 5 (windowed max_successes): rate-limit bypass via concurrency.
CREATE TABLE login_vuln (username TEXT PRIMARY KEY, attempts INT NOT NULL DEFAULT 0);
CREATE TABLE login_safe (username TEXT PRIMARY KEY, attempts INT NOT NULL DEFAULT 0);

-- Archetype 6 (single_transition): duplicate "confirm" state transition.
CREATE TABLE orders_vuln (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'pending');
CREATE TABLE orders_safe (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'pending');

-- Spring Boot corpus subset (Design/TEST_PLAN.md Layer 5: at minimum two
-- stacks). "_spring" tables are the same three archetypes reimplemented in
-- Spring Boot, kept separate from the Node app's tables so the two corpus
-- apps never share mutable state.
CREATE TABLE coupons_vuln_spring (code TEXT PRIMARY KEY, used BOOLEAN NOT NULL DEFAULT FALSE);
CREATE TABLE coupons_safe_spring (code TEXT PRIMARY KEY, used BOOLEAN NOT NULL DEFAULT FALSE);
CREATE TABLE inventory_vuln_spring (sku TEXT PRIMARY KEY, stock INT NOT NULL);
CREATE TABLE inventory_safe_spring (sku TEXT PRIMARY KEY, stock INT NOT NULL);
CREATE TABLE orders_vuln_spring (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'pending');
CREATE TABLE orders_safe_spring (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'pending');
