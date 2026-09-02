package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func newScopeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Create or validate an authorization scope (Design/API.md)",
		Long: "A Scope is RaceVeil's default-deny authorization boundary: the only hosts, ports,\n" +
			"and path prefixes it will ever touch, plus its concurrency/rate/total caps and its\n" +
			"proof ceiling. RaceVeil enforces the Scope technically; it does NOT verify that the\n" +
			"operator is actually authorized — authorized_by is provenance metadata you own.",
	}
	cmd.AddCommand(newScopeInitCmd(), newScopeCheckCmd())
	return cmd
}

func newScopeInitCmd() *cobra.Command {
	var target, out, authorizedBy string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Resolve a target, pin its IP, and write a default-deny scope.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := wiring.InitScope(target, out, authorizedBy)
			if err != nil {
				return &exitCodeErr{code: 5, err: err}
			}
			w := cmd.OutOrStdout()
			for host, ip := range result.Resolved {
				_, _ = fmt.Fprintf(w, "Resolved %s → %s (pinned).\n", host, ip)
			}
			_, _ = fmt.Fprintf(w, "Wrote default-deny scope to %s.\n", result.Path)
			_, _ = fmt.Fprintln(w, "Review it — especially hosts, path_prefixes, caps, and proof.max_successful_effects —")
			_, _ = fmt.Fprintln(w, "then run a scan. RaceVeil will only touch what this file allows.")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "target base URL, e.g. https://staging.shop.internal (required)")
	cmd.Flags().StringVar(&out, "out", "scope.yaml", "path to write the scope file to")
	cmd.Flags().StringVar(&authorizedBy, "authorized-by", "",
		"provenance string recorded in every report (e.g. \"jane@corp — ticket SEC-1421\"); not verified by RaceVeil")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func newScopeCheckCmd() *cobra.Command {
	var scopePath string
	var iAmAuthorized, noSafeMode bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate a scope and print exactly what RaceVeil would and would not touch",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := wiring.CheckScope(scopePath, !noSafeMode, iAmAuthorized)
			if err != nil {
				return &exitCodeErr{code: 5, err: err}
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintln(w, report.Effective)
			_, _ = fmt.Fprintf(w, "Authorized-by: %s   (operator-declared provenance; NOT verified by RaceVeil)\n", report.AuthorizedBy)
			_, _ = fmt.Fprintln(w, "\nWILL touch:")
			for _, line := range report.Allowed {
				_, _ = fmt.Fprintf(w, "  %s\n", line)
			}
			_, _ = fmt.Fprintln(w, "WILL NOT touch: everything else (default-deny), including:")
			for _, line := range report.Denied {
				_, _ = fmt.Fprintf(w, "  %s\n", line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopePath, "scope", "scope.yaml", "path to scope.yaml")
	cmd.Flags().BoolVar(&iAmAuthorized, "i-am-authorized", false, "acknowledge authorization for a public target")
	cmd.Flags().BoolVar(&noSafeMode, "no-safe-mode", false, "relax safe-mode; still requires --i-am-authorized")
	return cmd
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Capture or validate a Session (Design/API.md)",
		Long: "A Session is the authentication context a Workflow runs under. Session secrets live\n" +
			"in the session.json you supply and in memory during a run — they are redacted from\n" +
			"every persisted artifact, and .rv findings template them as ${ENV} placeholders.",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthCheckCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var recipePath, out string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run a login recipe and write the resulting Session to session.json",
		Long: "A login recipe describes how to obtain a Session: the request to send, and which\n" +
			"response cookies/fields to keep. Credentials are read from the recipe's referenced\n" +
			"environment variables, never inlined into the recipe file itself.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, session, err := wiring.RunAuthLogin(cmd.Context(), recipePath, out)
			if err != nil {
				return &exitCodeErr{code: 4, err: err}
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Captured session %q (isolation group %q, %d cookie(s), %d header(s)) → %s\n",
				session.ID, session.IsolationGroup, len(session.Cookies), len(session.Headers), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&recipePath, "recipe", "", "path to login.yaml (required)")
	cmd.Flags().StringVar(&out, "out", "session.json", "path to write the captured session to")
	_ = cmd.MarkFlagRequired("recipe")
	return cmd
}

func newAuthCheckCmd() *cobra.Command {
	var scopePath, authPath, probePath string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Confirm a Session is live and authenticated against the target",
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := wiring.CheckAuth(cmd.Context(), scopePath, authPath, probePath)
			if err != nil {
				return &exitCodeErr{code: 4, err: err}
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "Session %q (isolation group %q)\n", status.SessionID, status.IsolationGroup)
			_, _ = fmt.Fprintf(w, "Probe %s → %d\n", status.ProbeURL, status.Status)
			if status.Authenticated {
				_, _ = fmt.Fprintln(w, "Session looks live and authenticated.")
				return nil
			}
			_, _ = fmt.Fprintln(w, "Session does NOT look authenticated (the probe was redirected to a login or refused).")
			return &exitCodeErr{code: 4}
		},
	}
	cmd.Flags().StringVar(&scopePath, "scope", "scope.yaml", "path to scope.yaml")
	cmd.Flags().StringVar(&authPath, "auth", "session.json", "path to session.json")
	cmd.Flags().StringVar(&probePath, "probe", "/", "authenticated path to probe")
	return cmd
}
