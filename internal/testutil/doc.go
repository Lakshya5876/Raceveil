// Package testutil holds shared test fixtures and helpers used across
// RaceVeil's test suite — most importantly the frozen Oracle fixtures
// (baseline + concurrent-trial observations captured from real corpus runs,
// Design/TEST_PLAN.md Layer 3) under testdata/oracle/, which let Oracle
// classification logic be tested without live network traffic.
//
// CORE_FILES (docs/ARCHITECTURE_DECISIONS.md GOV-004): this package and its
// fixtures are what makes the Oracle's regression tests trustworthy.
package testutil
