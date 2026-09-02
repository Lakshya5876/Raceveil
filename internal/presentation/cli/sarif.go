package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

// SARIF 2.1.0 output for code-scanning dashboards (Design/API.md report
// --format sarif). Only LIKELY+ findings become SARIF results; SUSPECTED
// signals go to a separate notes property and are never asserted as
// vulnerabilities (Design/UX.md tone rules, Design/API.md report).
const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	toolName     = "RaceVeil"
	toolURI      = "https://github.com/Lakshya5876/Raceveil"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Results     []sarifResult     `json:"results"`
	Invocations []sarifInvocation `json:"invocations"`
	Properties  map[string]any    `json:"properties,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Version        string      `json:"version"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	ShortDescription     sarifText       `json:"shortDescription"`
	FullDescription      sarifText       `json:"fullDescription"`
	Help                 sarifText       `json:"help"`
	DefaultConfiguration sarifRuleConfig `json:"defaultConfiguration"`
	Properties           map[string]any  `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID     string          `json:"ruleId"`
	Level      string          `json:"level"`
	Message    sarifText       `json:"message"`
	Locations  []sarifLocation `json:"locations"`
	Properties map[string]any  `json:"properties,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
}

// ruleIDFor derives a stable SARIF rule id per invariant type, so a
// dashboard groups "duplicate coupon redemption" and "duplicate gift-card
// redemption" under one rule while keeping inventory overrun separate.
func ruleIDFor(inv domain.Invariant) string {
	switch inv.Type {
	case domain.InvariantUniqueness:
		return "raceveil/uniqueness-violation"
	case domain.InvariantMonotonicLimit:
		return "raceveil/monotonic-limit-violation"
	case domain.InvariantSingleTransition:
		return "raceveil/single-transition-violation"
	default:
		return "raceveil/max-successes-violation"
	}
}

func ruleNameFor(inv domain.Invariant) string {
	switch inv.Type {
	case domain.InvariantUniqueness:
		return "ConcurrentUniquenessViolation"
	case domain.InvariantMonotonicLimit:
		return "ConcurrentMonotonicLimitViolation"
	case domain.InvariantSingleTransition:
		return "ConcurrentSingleTransitionViolation"
	default:
		return "ConcurrentMaxSuccessesViolation"
	}
}

// sarifLevel maps Confidence to a SARIF level. CONFIRMED is an error;
// LIKELY is a warning; SUSPECTED never reaches SARIF results at all.
func sarifLevel(c domain.Confidence) string {
	if c == domain.Confirmed {
		return "error"
	}
	return "warning"
}

// writeSARIF renders findings as a SARIF 2.1.0 log. suspected carries the
// count of SUSPECTED-only experiments, recorded as run properties rather
// than results so a CI dashboard never shows them as vulnerabilities.
func writeSARIF(w io.Writer, findings []domain.Finding, suspected int, version string) error {
	rules := make([]sarifRule, 0, 4)
	seenRule := make(map[string]bool)
	results := make([]sarifResult, 0, len(findings))

	for _, f := range findings {
		if f.Confidence == domain.Suspected {
			continue // never assert a SUSPECTED signal as a finding
		}
		id := ruleIDFor(f.Invariant)
		if !seenRule[id] {
			seenRule[id] = true
			rules = append(rules, sarifRule{
				ID:               id,
				Name:             ruleNameFor(f.Invariant),
				ShortDescription: sarifText{Text: "Concurrency-induced integrity violation (" + string(f.Invariant.Type) + ")"},
				FullDescription: sarifText{Text: "Concurrent execution of this workflow produced more successful effects than its " +
					string(f.Invariant.Type) + " invariant permits. The same workflow run sequentially respects the limit, so this is a " +
					"concurrency-induced violation rather than an ordinary logic bug."},
				Help: sarifText{Text: "Reproduce with `raceveil replay " + findingPath(f) +
					"`; independently re-confirm with `raceveil verify " + findingPath(f) + "`."},
				DefaultConfiguration: sarifRuleConfig{Level: "error"},
				Properties: map[string]any{
					"invariantType": string(f.Invariant.Type),
					"tags":          []string{"security", "concurrency", "integrity"},
				},
			})
		}

		rep := f.ObservedAtCreation.Reproduction
		results = append(results, sarifResult{
			RuleID: id,
			Level:  sarifLevel(f.Confidence),
			Message: sarifText{Text: fmt.Sprintf(
				"%s: expected at most %d successful effect(s), observed %d under N=%d concurrent requests. "+
					"Reproduced in %d/%d independent trials (Wilson95 lower bound %.3f). Confidence: %s.",
				wiring.ActRequestLine(f.Workflow), f.Invariant.Value, f.ObservedAtCreation.S, f.ConcurrencyN,
				rep.RViolations, rep.KIndependent, rep.Wilson95[0], f.Confidence)},
			Locations: []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: locationURI(f)},
			}}},
			Properties: map[string]any{
				"confidence":             string(f.Confidence),
				"confidenceBandsVersion": f.ObservedAtCreation.ConfidenceBandsVersion,
				"severity":               f.Severity.Label,
				"severityBasis":          f.Severity.Basis,
				"invariantType":          string(f.Invariant.Type),
				"invariantValue":         f.Invariant.Value,
				"observedSuccesses":      f.ObservedAtCreation.S,
				"minimizedConcurrencyN":  f.ConcurrencyN,
				"stateIndependence":      string(f.StateIndependence),
				"corroboratingLevels":    f.Oracle.CorroboratingLevels,
				"rViolations":            rep.RViolations,
				"kIndependent":           rep.KIndependent,
				"wilson95Lower":          rep.Wilson95[0],
				"reproductionArtifact":   findingPath(f),
			},
		})
	}

	log := sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name: toolName, InformationURI: toolURI, Version: version, Rules: rules,
			}},
			Results:     results,
			Invocations: []sarifInvocation{{ExecutionSuccessful: true}},
			Properties: map[string]any{
				"suspectedOnlySignals": suspected,
				"note": "SUSPECTED signals are recorded here as a count only and are never emitted as SARIF results — " +
					"RaceVeil does not assert them as vulnerabilities.",
			},
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

// locationURI gives a dashboard something stable to group on. RaceVeil has
// no source file to point at (it is black-box by design), so the endpoint
// itself is the artifact.
func locationURI(f domain.Finding) string {
	if spec, ok := f.Workflow.RequestByID(f.Workflow.ActRequest); ok {
		return spec.URL
	}
	return f.Target
}
