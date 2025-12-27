# Consolidated Project Review Findings & Scope Reduction Plan

This document summarizes the findings from the Phase 2 project review and outlines the architectural pivot toward a context-saving Smart MCP Router.

## 1. Consolidated Findings Table

| Issue | Severity | Impact | Proposed Fix | File / Module |
| :--- | :--- | :--- | :--- | :--- |
| **Tool Discovery Policy Block** | High | Prevents LLM from seeing available tools due to strict security/context policies. | Implement a deterministic Tool Registry that provides a static list of tools to the LLM instead of dynamic discovery. | `agent/router/` |
| **Context Bloat (Schemas)** | Medium | High token usage and latency in LLM calls. | Implement schema compression (removing descriptions/optional fields) and Top-K tool selection based on intent. | `agent/mcp/` |
| **Repository Duplication** | Low | Developer confusion and accidental context pollution for AI assistants. | Archive `stage*` and `task*` directories; remove or consolidate `.worktrees`. | Root Repository |
| **Non-Deterministic Truncation** | Medium | Loss of critical conversation history or tool outputs. | Implement strict token budgets and deterministic truncation (keeping system prompt + latest N messages). | `agent/context/` |
| **Lack of Fallback Router** | Medium | System failure if primary MCP server is unreachable. | Implement a fallback mechanism to secondary MCP nodes or a local "safe-mode" toolset. | `agent/router/` |

## 2. Deep Dive: Tool Discovery Policy Block
*Reference: docs/functions-router-summary.md*

### Problem
Current implementations often rely on the LLM to "discover" tools by querying the MCP server. This creates a "policy-block" where the security layer or context-window manager restricts the discovery call, leaving the LLM without the necessary tool definitions to fulfill requests.

### Architectural Solution: The Smart Router
To resolve this while saving context, the project will pivot to a **Deterministic Registry Approach**:

1.  **Explicit Registry**: Instead of `list_tools` calls at runtime, the router maintains a local registry of available tool signatures.
2.  **Intent-Based Selection**: A lightweight classifier (or keyword matcher) selects the Top-K (e.g., 3-5) most relevant tools for the current user prompt.
3.  **Schema Compression**: Tool JSON schemas are stripped of redundant metadata before being injected into the system prompt.
4.  **Fallback Mechanism**:
    *   **Primary**: Direct routing to the specific MCP server.
    *   **Secondary**: If the primary server fails, the router falls back to a "Core Tools" set (e.g., filesystem, basic shell) to attempt recovery or provide a graceful error.

## 3. Context Saving Strategy

*   **Registry vs. Discovery**: By using a registry, we eliminate the 1-2 round trips usually required for tool discovery.
*   **Deterministic Truncation**: We will enforce a 20% buffer in the context window. If the limit is reached, the router will truncate tool outputs first, then middle-of-conversation history, preserving the System Prompt and the last 3 turns.
*   **Budgeting**:
    *   System Prompt + Compressed Tools: Max 2000 tokens.
    *   Active Context: Max 4000 tokens.
    *   Remaining: Reserved for LLM generation.

## 4. Repository Cleanup Plan
The following directories are marked for archival to reduce the "noise" in the project context:
*   `stage1/` through `stageN/` (Move to `archive/stages/`)
*   `task1/` through `taskN/` (Move to `archive/tasks/`)
*   Cleanup of `.worktrees/` to ensure only the active development branch is indexed.

*Note: These file movements will be performed in a dedicated PR following the approval of this consolidation.*