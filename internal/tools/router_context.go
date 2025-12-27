package tools

import (
	"os"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/router"
)

// prepareRouterContext prepares and enriches the router context with:
// - extracted structured context (URLs/IDs)
// - client prefixes from input
// - client configuration from environment
func prepareRouterContext(input string, ctx map[string]any) (string, map[string]any) {
	if ctx == nil {
		ctx = make(map[string]any)
	}

	// Deterministic context extraction (URLs/IDs) before any planning.
	if extracted := router.ExtractStructuredContext(input); len(extracted) > 0 {
		for k, v := range extracted {
			if _, exists := ctx[k]; !exists {
				ctx[k] = v
			}
		}
	}

	// Convenience: allow prefixing the query with `jira <client>` to route Jira calls to that client.
	if client, rest := extractJiraClientPrefix(input); client != "" {
		ctx["jira_client"] = client
		if rest != "" {
			input = rest
		}
	}

	// Convenience: allow prefixing the query with `confluence <client>` to route Confluence calls to that client.
	if client, rest := extractConfluenceClientPrefix(input); client != "" {
		ctx["confluence_client"] = client
		if rest != "" {
			input = rest
		}
	}

	// Convenience: allow prefixing the query with `grafana <client>` to route Grafana calls to that client.
	if client, rest := extractGrafanaClientPrefix(input); client != "" {
		ctx["grafana_client"] = client
		if rest != "" {
			input = rest
		}
	}

	// Provide configured Jira clients (no secrets) to the planner so it can pick the right instance.
	if clients := jiraPublicClientsFromEnv(); len(clients) > 0 {
		ctx["jira_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("JIRA_DEFAULT_CLIENT")); def != "" {
		ctx["jira_default_client"] = def
	}

	// Provide configured Confluence clients (no secrets) to the planner so it can pick the right instance.
	if clients := confluencePublicClientsFromEnv(); len(clients) > 0 {
		ctx["confluence_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("CONFLUENCE_DEFAULT_CLIENT")); def != "" {
		ctx["confluence_default_client"] = def
	}

	// Provide configured Grafana clients (no secrets) to the planner so it can pick the right instance.
	if clients := grafanaPublicClientsFromEnv(); len(clients) > 0 {
		ctx["grafana_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("GRAFANA_DEFAULT_CLIENT")); def != "" {
		ctx["grafana_default_client"] = def
	}

	return input, ctx
}

// applyContextToPlan applies extracted context and client preferences to the plan steps.
// This makes instance selection deterministic (do not rely on the model to thread it through).
func (h *Handler) applyContextToPlan(plan *router.ModelPlan, ctx map[string]any) {
	if plan == nil || len(plan.Steps) == 0 || ctx == nil {
		return
	}

	// Make Jira instance selection deterministic.
	h.applyJiraClientToPlan(plan, ctx)
	// Make Confluence instance selection deterministic.
	h.applyConfluenceClientToPlan(plan, ctx)
	// Make Grafana instance selection deterministic.
	h.applyGrafanaClientToPlan(plan, ctx)
	// Make URL/ID context injection deterministic.
	h.applyExtractedContextToPlan(plan, ctx)
}
