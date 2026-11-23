package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command {
		return []*cobra.Command{a.reviewCommand()}
	})
}

// The review commands are driven by agents, so they always print the server's
// JSON response, with or without --json.

var reviewCriteriaPattern = regexp.MustCompile(`^v\d{1,4}$`)

func (a *App) reviewCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "review",
		Short: "Staff tools for card review (the server decides who may use them)",
		Long: `Staff tools for card review submissions: list the queue, read a submission,
and record an advisory suggestion or flags for one. Suggestions do not decide
a review. The server decides which accounts may use each command.

Every command prints the server's JSON response.`,
	}
	c.AddCommand(a.reviewQueue(), a.reviewShow(), a.reviewSuggest(), a.reviewTag(), a.reviewReport())
	return c
}

func (a *App) reviewQueue() *cobra.Command {
	var scope, criteria string
	var unshadowed, count bool
	var limit int
	c := &cobra.Command{
		Use:   "queue",
		Short: "List review submissions",
		Long: `Lists review submissions. --scope pending (default) lists submissions waiting
for a decision; recent lists those submitted in the last 7 days, decided or not.
--unshadowed keeps only submissions you have not given a verdict for yet (for
the --criteria version, when given). --count prints {"count": n} instead of items.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if criteria != "" && !reviewCriteriaPattern.MatchString(criteria) {
				return output.Exitf(2, "--criteria must look like v1")
			}
			q := url.Values{"scope": {scope}, "limit": {strconv.Itoa(limit)}}
			if unshadowed {
				q.Set("unshadowed", "1")
			}
			if criteria != "" {
				q.Set("criteria", criteria)
			}
			if count {
				q.Set("count", "1")
			}
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/queue?"+q.Encode(), nil)
		},
	}
	c.Flags().StringVar(&scope, "scope", "pending", "pending or recent")
	c.Flags().BoolVar(&unshadowed, "unshadowed", false, "only submissions you have not given a verdict for")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version for --unshadowed, e.g. v1")
	c.Flags().BoolVar(&count, "count", false, "print only the number of matching submissions")
	c.Flags().IntVar(&limit, "limit", 50, "maximum items (1-200)")
	return c
}

func (a *App) reviewShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show <submissionId>",
		Short: "Read one review submission with its card and the allowed suggestion codes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.reviewCall(cmd, http.MethodGet, "/review-ai/submissions/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (a *App) reviewSuggest() *cobra.Command {
	var file, criteria, model, verdict, note, handoffCause string
	var reasons, flags []string
	var confidence, costUSD float64
	var inputTokens, outputTokens, latencyMS int64
	c := &cobra.Command{
		Use:   "suggest <submissionId>",
		Short: "Record an advisory suggestion for a review submission",
		Long: `Records your suggestion for a submission: a verdict (pass, reject or handoff),
reason and flag codes, confidence and a short note. It is advisory only and does
not decide the review. Sending again for the same submission and criteria
version replaces your earlier suggestion.

Use flags, or --file with a JSON body (- reads stdin). Flags given on the command
line replace the matching fields of the file. Allowed codes are listed by
review show.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if file != "" {
				var err error
				if body, err = a.readReviewBody(file); err != nil {
					return err
				}
			}
			f := cmd.Flags()
			set := func(flag, key string, v any) {
				if f.Changed(flag) {
					body[key] = v
				}
			}
			set("criteria", "criteriaVersion", criteria)
			set("model", "model", model)
			set("verdict", "verdict", verdict)
			set("reason", "reasons", nonNilStrings(reasons))
			set("flag", "flags", nonNilStrings(flags))
			set("confidence", "confidence", confidence)
			set("note", "note", note)
			set("handoff-cause", "handoffCause", handoffCause)
			set("input-tokens", "inputTokens", inputTokens)
			set("output-tokens", "outputTokens", outputTokens)
			set("cost-usd", "costUsd", costUSD)
			set("latency-ms", "latencyMs", latencyMS)
			if file == "" {
				for _, key := range []string{"reasons", "flags"} {
					if _, ok := body[key]; !ok {
						body[key] = []string{}
					}
				}
			}
			if err := checkReviewCriteria(body); err != nil {
				return err
			}
			switch body["verdict"] {
			case "pass", "reject", "handoff":
			default:
				return output.Exitf(2, "verdict must be pass, reject or handoff")
			}
			return a.reviewCall(cmd, http.MethodPut, "/review-ai/submissions/"+url.PathEscape(args[0])+"/suggestion", body)
		},
	}
	c.Flags().StringVar(&file, "file", "", "JSON body to send ('-' for stdin)")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	c.Flags().StringVar(&model, "model", "", "model that produced the suggestion")
	c.Flags().StringVar(&verdict, "verdict", "", "pass, reject or handoff")
	c.Flags().StringArrayVar(&reasons, "reason", nil, "reason code (repeatable)")
	c.Flags().StringArrayVar(&flags, "flag", nil, "flag code (repeatable)")
	c.Flags().Float64Var(&confidence, "confidence", 0, "confidence from 0 to 1")
	c.Flags().StringVar(&note, "note", "", "short note (up to 400 characters)")
	c.Flags().StringVar(&handoffCause, "handoff-cause", "", "why the submission is handed to a person: unsure, refusal, invalid_output, timeout, error or too_large")
	c.Flags().Int64Var(&inputTokens, "input-tokens", 0, "model input tokens")
	c.Flags().Int64Var(&outputTokens, "output-tokens", 0, "model output tokens")
	c.Flags().Float64Var(&costUSD, "cost-usd", 0, "model cost in US dollars")
	c.Flags().Int64Var(&latencyMS, "latency-ms", 0, "model latency in milliseconds")
	return c
}

func (a *App) reviewTag() *cobra.Command {
	var criteria, model string
	var flags []string
	c := &cobra.Command{
		Use:   "tag <submissionId>",
		Short: "Set flag codes on a review submission without changing your verdict",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"criteriaVersion": criteria, "model": model, "flags": nonNilStrings(flags)}
			if err := checkReviewCriteria(body); err != nil {
				return err
			}
			return a.reviewCall(cmd, http.MethodPut, "/review-ai/submissions/"+url.PathEscape(args[0])+"/tags", body)
		},
	}
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	c.Flags().StringVar(&model, "model", "", "model that produced the flags")
	c.Flags().StringArrayVar(&flags, "flag", nil, "flag code (repeatable)")
	return c
}

func (a *App) reviewReport() *cobra.Command {
	var from, to int64
	var criteria string
	c := &cobra.Command{
		Use:   "report",
		Short: "Summarize suggestions by criteria version over a time range",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if criteria != "" && !reviewCriteriaPattern.MatchString(criteria) {
				return output.Exitf(2, "--criteria must look like v1")
			}
			q := url.Values{}
			if cmd.Flags().Changed("from") {
				q.Set("from", strconv.FormatInt(from, 10))
			}
			if cmd.Flags().Changed("to") {
				q.Set("to", strconv.FormatInt(to, 10))
			}
			if criteria != "" {
				q.Set("criteria", criteria)
			}
			path := "/review-ai/report"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			return a.reviewCall(cmd, http.MethodGet, path, nil)
		},
	}
	c.Flags().Int64Var(&from, "from", 0, "start time in Unix milliseconds")
	c.Flags().Int64Var(&to, "to", 0, "end time in Unix milliseconds")
	c.Flags().StringVar(&criteria, "criteria", "", "criteria version, e.g. v1")
	return c
}

// reviewCall sends one signed-in community request and prints the response.
func (a *App) reviewCall(cmd *cobra.Command, method, path string, body any) error {
	if err := a.RequireAuth(); err != nil {
		return err
	}
	var raw json.RawMessage
	if err := a.Client().SiteDo(cmd.Context(), method, path, body, &raw); err != nil {
		return withErrorField(err)
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return a.Out.JSONValue(raw)
}

// withErrorField copies the response's "field" into the error detail so the
// printed error names the rejected field next to the code.
func withErrorField(err error) error {
	var e *api.Error
	if !errors.As(err, &e) || e.Detail != nil || len(e.Envelope) == 0 {
		return err
	}
	var env struct {
		Field string `json:"field"`
	}
	if json.Unmarshal(e.Envelope, &env) == nil && env.Field != "" {
		e.Detail = map[string]any{"field": env.Field}
	}
	return err
}

func (a *App) readReviewBody(path string) (map[string]any, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(a.In)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, output.Exit(2, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil || body == nil {
		return nil, output.Exitf(2, "--file must hold one JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, output.Exitf(2, "--file must hold one JSON object")
	}
	return body, nil
}

func checkReviewCriteria(body map[string]any) error {
	if s, ok := body["criteriaVersion"].(string); !ok || !reviewCriteriaPattern.MatchString(s) {
		return output.Exitf(2, "criteria version must look like v1 (--criteria)")
	}
	return nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
