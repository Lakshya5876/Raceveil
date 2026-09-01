// RaceVeil Phase 2 seeded corpus — Node/Express + PostgreSQL.
// Six race archetypes (Design/TEST_PLAN.md Layer 5), each with a vulnerable
// (check-then-act, two separate queries, no lock) and a synchronized-safe
// (single atomic statement, or an explicit transaction with row locking)
// twin. Local-only: binds 127.0.0.1, talks to the local-only Postgres
// container in corpus/docker-compose.yml. Nothing here is deployed or
// billed anywhere.
'use strict';

const express = require('express');
const { Pool } = require('pg');
const crypto = require('crypto');

const PORT = process.env.PORT || 4000;
const pool = new Pool({
  host: '127.0.0.1',
  port: 55432,
  user: 'raceveil',
  password: 'raceveil_corpus_local_only',
  database: 'raceveil_corpus',
});

const app = express();
app.use(express.json());

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const randomId = () => crypto.randomBytes(8).toString('hex');

// Widens the check-then-act race window to a size reliably larger than
// scheduler jitter, mirroring internal/testutil/fixtureserver's own
// documented rationale — it does not change what makes the endpoint
// vulnerable (the two queries are still unlocked and separately visible).
const RACE_WINDOW_MS = 8;

// ---------------------------------------------------------------------
// Archetype 1 (max_successes): check-then-act coupon redemption.
// ---------------------------------------------------------------------

app.post('/coupon/issue', async (req, res, next) => {
  try {
    const code = randomId();
    await pool.query('INSERT INTO coupons_vuln (code, used) VALUES ($1, false)', [code]);
    res.json({ code });
  } catch (err) { next(err); }
});

app.post('/coupon/redeem', async (req, res, next) => {
  try {
    const { code } = req.body;
    const check = await pool.query('SELECT used FROM coupons_vuln WHERE code = $1', [code]);
    if (check.rows.length === 0 || check.rows[0].used) {
      return res.status(409).json({ error: 'already used' });
    }
    await sleep(RACE_WINDOW_MS);
    await pool.query('UPDATE coupons_vuln SET used = true WHERE code = $1', [code]);
    res.json({ status: 'redeemed', receipt_id: randomId() });
  } catch (err) { next(err); }
});

app.post('/coupon/issue-safe', async (req, res, next) => {
  try {
    const code = randomId();
    await pool.query('INSERT INTO coupons_safe (code, used) VALUES ($1, false)', [code]);
    res.json({ code });
  } catch (err) { next(err); }
});

app.post('/coupon/redeem-safe', async (req, res, next) => {
  try {
    const { code } = req.body;
    const result = await pool.query(
      'UPDATE coupons_safe SET used = true WHERE code = $1 AND used = false RETURNING code',
      [code],
    );
    if (result.rowCount === 0) {
      return res.status(409).json({ error: 'already used' });
    }
    res.json({ status: 'redeemed', receipt_id: randomId() });
  } catch (err) { next(err); }
});

// ---------------------------------------------------------------------
// Archetype 2 (uniqueness): duplicate resource creation on signup.
// vuln checks existence then inserts (two statements); safe relies on the
// column's UNIQUE constraint plus a caught constraint-violation error.
// ---------------------------------------------------------------------

app.post('/signup/issue-username', async (_req, res) => {
  res.json({ username: 'user_' + randomId() });
});

app.post('/signup', async (req, res, next) => {
  try {
    const { username } = req.body;
    const existing = await pool.query('SELECT id FROM signups_vuln WHERE username = $1', [username]);
    if (existing.rows.length > 0) {
      return res.status(409).json({ error: 'username taken' });
    }
    await sleep(RACE_WINDOW_MS);
    const inserted = await pool.query(
      'INSERT INTO signups_vuln (username) VALUES ($1) RETURNING id',
      [username],
    );
    res.json({ status: 'created', user_id: inserted.rows[0].id });
  } catch (err) { next(err); }
});

app.post('/signup-safe', async (req, res, next) => {
  try {
    const { username } = req.body;
    const inserted = await pool.query(
      'INSERT INTO signups_safe (username) VALUES ($1) RETURNING id',
      [username],
    );
    res.json({ status: 'created', user_id: inserted.rows[0].id });
  } catch (err) {
    if (err.code === '23505') { // unique_violation
      return res.status(409).json({ error: 'username taken' });
    }
    next(err);
  }
});

// ---------------------------------------------------------------------
// Archetype 3 (max_successes): duplicate redemption of a one-time gift
// card — distinct resource from archetype 1, response carries a balance
// field for Level 2 body-differential testing.
// ---------------------------------------------------------------------

app.post('/giftcard/issue', async (req, res, next) => {
  try {
    const code = randomId();
    await pool.query('INSERT INTO giftcards_vuln (code, redeemed, balance) VALUES ($1, false, 50)', [code]);
    res.json({ code });
  } catch (err) { next(err); }
});

app.post('/giftcard/redeem', async (req, res, next) => {
  try {
    const { code } = req.body;
    const check = await pool.query('SELECT redeemed, balance FROM giftcards_vuln WHERE code = $1', [code]);
    if (check.rows.length === 0 || check.rows[0].redeemed) {
      return res.status(409).json({ error: 'already redeemed' });
    }
    await sleep(RACE_WINDOW_MS);
    await pool.query('UPDATE giftcards_vuln SET redeemed = true WHERE code = $1', [code]);
    res.json({ status: 'redeemed', receipt_id: randomId(), balance: check.rows[0].balance });
  } catch (err) { next(err); }
});

app.post('/giftcard/issue-safe', async (req, res, next) => {
  try {
    const code = randomId();
    await pool.query('INSERT INTO giftcards_safe (code, redeemed, balance) VALUES ($1, false, 50)', [code]);
    res.json({ code });
  } catch (err) { next(err); }
});

app.post('/giftcard/redeem-safe', async (req, res, next) => {
  try {
    const { code } = req.body;
    const result = await pool.query(
      'UPDATE giftcards_safe SET redeemed = true WHERE code = $1 AND redeemed = false RETURNING balance',
      [code],
    );
    if (result.rowCount === 0) {
      return res.status(409).json({ error: 'already redeemed' });
    }
    res.json({ status: 'redeemed', receipt_id: randomId(), balance: result.rows[0].balance });
  } catch (err) { next(err); }
});

// ---------------------------------------------------------------------
// Archetype 4 (monotonic_limit): inventory overrun on the last unit.
// vuln: SELECT stock then UPDATE (two statements). safe: an explicit
// transaction with SELECT ... FOR UPDATE, the textbook Postgres pattern.
// ---------------------------------------------------------------------

app.post('/inventory/restock', async (req, res, next) => {
  try {
    const sku = 'sku_' + randomId();
    await pool.query('INSERT INTO inventory_vuln (sku, stock) VALUES ($1, 1)', [sku]);
    res.json({ sku });
  } catch (err) { next(err); }
});

app.post('/inventory/reserve', async (req, res, next) => {
  try {
    const { sku } = req.body;
    const check = await pool.query('SELECT stock FROM inventory_vuln WHERE sku = $1', [sku]);
    if (check.rows.length === 0 || check.rows[0].stock <= 0) {
      return res.status(409).json({ error: 'out of stock' });
    }
    await sleep(RACE_WINDOW_MS);
    await pool.query('UPDATE inventory_vuln SET stock = stock - 1 WHERE sku = $1', [sku]);
    res.json({ status: 'reserved', receipt_id: randomId() });
  } catch (err) { next(err); }
});

app.post('/inventory/restock-safe', async (req, res, next) => {
  try {
    const sku = 'sku_' + randomId();
    await pool.query('INSERT INTO inventory_safe (sku, stock) VALUES ($1, 1)', [sku]);
    res.json({ sku });
  } catch (err) { next(err); }
});

app.post('/inventory/reserve-safe', async (req, res, next) => {
  const client = await pool.connect();
  try {
    const { sku } = req.body;
    await client.query('BEGIN');
    const check = await client.query('SELECT stock FROM inventory_safe WHERE sku = $1 FOR UPDATE', [sku]);
    if (check.rows.length === 0 || check.rows[0].stock <= 0) {
      await client.query('ROLLBACK');
      return res.status(409).json({ error: 'out of stock' });
    }
    await client.query('UPDATE inventory_safe SET stock = stock - 1 WHERE sku = $1', [sku]);
    await client.query('COMMIT');
    res.json({ status: 'reserved', receipt_id: randomId() });
  } catch (err) {
    await client.query('ROLLBACK');
    next(err);
  } finally {
    client.release();
  }
});

// ---------------------------------------------------------------------
// Archetype 5 (windowed max_successes): rate-limit bypass via
// concurrency — at most 3 login attempts allowed per username.
// ---------------------------------------------------------------------

app.post('/login/issue-username', async (req, res, next) => {
  try {
    const username = 'login_' + randomId();
    await pool.query('INSERT INTO login_vuln (username, attempts) VALUES ($1, 0)', [username]);
    res.json({ username });
  } catch (err) { next(err); }
});

app.post('/login/attempt', async (req, res, next) => {
  try {
    const { username } = req.body;
    const check = await pool.query('SELECT attempts FROM login_vuln WHERE username = $1', [username]);
    if (check.rows.length === 0 || check.rows[0].attempts >= 3) {
      return res.status(429).json({ error: 'rate limited' });
    }
    await sleep(RACE_WINDOW_MS);
    await pool.query('UPDATE login_vuln SET attempts = attempts + 1 WHERE username = $1', [username]);
    res.json({ status: 'attempted', receipt_id: randomId() });
  } catch (err) { next(err); }
});

app.post('/login/issue-username-safe', async (req, res, next) => {
  try {
    const username = 'login_' + randomId();
    await pool.query('INSERT INTO login_safe (username, attempts) VALUES ($1, 0)', [username]);
    res.json({ username });
  } catch (err) { next(err); }
});

app.post('/login/attempt-safe', async (req, res, next) => {
  try {
    const { username } = req.body;
    const result = await pool.query(
      'UPDATE login_safe SET attempts = attempts + 1 WHERE username = $1 AND attempts < 3 RETURNING attempts',
      [username],
    );
    if (result.rowCount === 0) {
      return res.status(429).json({ error: 'rate limited' });
    }
    res.json({ status: 'attempted', receipt_id: randomId() });
  } catch (err) { next(err); }
});

// ---------------------------------------------------------------------
// Archetype 6 (single_transition): duplicate "confirm" state transition.
// ---------------------------------------------------------------------

app.post('/order/create', async (req, res, next) => {
  try {
    const id = 'order_' + randomId();
    await pool.query("INSERT INTO orders_vuln (id, status) VALUES ($1, 'pending')", [id]);
    res.json({ id });
  } catch (err) { next(err); }
});

app.post('/order/confirm', async (req, res, next) => {
  try {
    const { id } = req.body;
    const check = await pool.query('SELECT status FROM orders_vuln WHERE id = $1', [id]);
    if (check.rows.length === 0 || check.rows[0].status !== 'pending') {
      return res.status(409).json({ error: 'not pending' });
    }
    await sleep(RACE_WINDOW_MS);
    await pool.query("UPDATE orders_vuln SET status = 'confirmed' WHERE id = $1", [id]);
    res.json({ status: 'confirmed', receipt_id: randomId() });
  } catch (err) { next(err); }
});

app.post('/order/create-safe', async (req, res, next) => {
  try {
    const id = 'order_' + randomId();
    await pool.query("INSERT INTO orders_safe (id, status) VALUES ($1, 'pending')", [id]);
    res.json({ id });
  } catch (err) { next(err); }
});

app.post('/order/confirm-safe', async (req, res, next) => {
  try {
    const { id } = req.body;
    const result = await pool.query(
      "UPDATE orders_safe SET status = 'confirmed' WHERE id = $1 AND status = 'pending' RETURNING id",
      [id],
    );
    if (result.rowCount === 0) {
      return res.status(409).json({ error: 'not pending' });
    }
    res.json({ status: 'confirmed', receipt_id: randomId() });
  } catch (err) { next(err); }
});

// ---------------------------------------------------------------------

app.use((err, _req, res, _next) => {
  // eslint-disable-next-line no-console
  console.error(err);
  res.status(500).json({ error: 'internal error' });
});

app.listen(PORT, '127.0.0.1', () => {
  // eslint-disable-next-line no-console
  console.log(`raceveil corpus (node) listening on 127.0.0.1:${PORT}`);
});
