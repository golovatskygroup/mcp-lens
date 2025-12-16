package router

import (
	"encoding/json"
	"fmt"
)

func BuildPlanSystemPrompt() string {
	return "You are a tool-routing model. Return ONLY valid JSON. No markdown. No extra text."
}

func BuildPlanUserPrompt(userInput string, ctx map[string]any, catalog []ToolCatalogItem, maxSteps int) (string, error) {
	instructions := map[string]any{
		"pr_review_workflow": []string{
			"For comprehensive PR reviews, prefer fetch_complete_pr_diff (saves full diff to file) over get_pull_request_diff (returns chunks).",
			"For large PRs (>30 files), use fetch_complete_pr_files to get all files with auto-pagination.",
			"The system will auto-continue pagination if has_next=true in results, so you don't need to plan multiple pagination steps.",
			"Use get_pull_request_summary first to understand PR scope before fetching full diff.",
			"For CI debugging, use github_list_workflow_runs -> github_list_workflow_jobs -> github_download_job_logs to fetch failed job logs as artifacts.",
		},
		"jira_workflow": []string{
			"For Jira tasks, start with jira_search_issues using JQL to find the right issues, then use jira_get_issue for details.",
			"For fast ticket context, prefer jira_get_issue_bundle (issue + comments, optional changelog).",
			"If the user asks to export Jira issues to local files, use jira_export_tasks (it will save markdown files and optionally expand Confluence links).",
			"For large result sets, use pagination with startAt/maxResults.",
			"Use jira_list_projects to discover project keys (if needed) and jira_get_myself to validate authentication.",
			"If context.jira_client is set (from `jira <client>` prefix), always set args.client for all jira_* tool calls, unless args.base_url is explicitly set.",
			"Available Jira client aliases (if configured) are in context.jira_clients; default alias (if set) is context.jira_default_client.",
		},
		"confluence_workflow": []string{
			"For Confluence tasks, prefer confluence_search_cql (CQL) to find pages by text/title/space, then use confluence_get_page for full details.",
			"To find a page by exact title in a space, use confluence_get_page_by_title (space_key + title).",
			"Use confluence_list_spaces to discover available space keys (if needed).",
			"For page trees and docs packs, use confluence_get_page_children; for attachments, use confluence_list_page_attachments and optionally confluence_download_attachment.",
			"For large result sets, use cursor/start pagination. Auto-pagination is enabled when a tool returns has_next=true.",
			"If context.confluence_client is set (from `confluence <client>` prefix), always set args.client for all confluence_* tool calls, unless args.base_url is explicitly set.",
			"Available Confluence client aliases (if configured) are in context.confluence_clients; default alias (if set) is context.confluence_default_client.",
		},
		"grafana_workflow": []string{
			"For Grafana tasks, start with grafana_search to find dashboards/folders by query/tags, then use grafana_get_dashboard or grafana_get_folder for details.",
			"For dashboard reviews/analysis, prefer grafana_get_dashboard_summary (smaller output with panels/queries/variables) over grafana_get_dashboard (full JSON).",
			"If the user provides a Grafana dashboard URL like https://<host>/d/<uid>/..., extract uid and pass it; also infer base_url (scheme+host) and org_id (from orgId query param) when possible.",
			"For alert inventory, use grafana_list_alert_rules (unified alerting) and/or grafana_list_alerts (legacy).",
			"Use grafana_get_current_user to validate authentication and permissions.",
			"Use grafana_list_datasources and grafana_get_datasource to discover data source metadata.",
			"For annotations, use grafana_query_annotations and optionally grafana_list_annotation_tags.",
			"If context.grafana_client is set (from `grafana <client>` prefix), always set args.client for all grafana_* tool calls, unless args.base_url is explicitly set.",
			"Available Grafana client aliases (if configured) are in context.grafana_clients; default alias (if set) is context.grafana_default_client.",
		},
		"pagination":  "Auto-pagination is enabled. If a tool returns has_next=true, the system will automatically fetch the next page/chunk.",
		"file_output": "Tools like fetch_complete_pr_diff save results to files and return file paths. The LLM client can then read these files.",
	}

	if devModeEnabled() {
		instructions["dev_workflow"] = []string{
			"When the user asks to add/generate a new local tool, call dev_scaffold_tool with tool_name/tool_description/input_schema and a clear spec.",
			"Prefer use_worktree=true (isolated worktree) and run_tests=false by default; keep remote operations read-only.",
		}
	}

	payload := map[string]any{
		"task":      userInput,
		"context":   ctx,
		"max_steps": maxSteps,
		"policy": map[string]any{
			"read_only": true,
			"must_not":  []string{"create", "update", "merge", "delete", "write", "push"},
		},
		"instructions": instructions,
		"tools":        catalog,
		"response_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":           map[string]any{"type": "string"},
							"source":         map[string]any{"type": "string", "enum": []string{"local", "upstream"}},
							"args":           map[string]any{"type": "object"},
							"reason":         map[string]any{"type": "string"},
							"parallel_group": map[string]any{"type": "string"},
						},
						"required": []string{"name", "source", "args"},
					},
				},
				"final_answer_needed": map[string]any{"type": "boolean"},
			},
			"required": []string{"steps", "final_answer_needed"},
		},
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Generate a tool execution plan as JSON.\n\n%s", string(b)), nil
}

func BuildSummarizeSystemPrompt() string {
	return "You summarize tool results for a human. Be concise and accurate. Return plain text (no JSON, no markdown code fences)."
}

func BuildSummarizeUserPrompt(userInput string, res RouterResult) (string, error) {
	payload := map[string]any{
		"task":           userInput,
		"executed_steps": TruncateExecutedStepsForLLM(res.ExecutedSteps),
		"manifest":       res.Manifest,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Write a final answer for the user based on executed tool results.\n\n%s", string(b)), nil
}
