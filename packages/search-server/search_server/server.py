"""MCP server for web search via OpenRouter Perplexity Sonar Pro."""

import json
import os
import sys
from typing import Any

import httpx
from dotenv import load_dotenv
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import (
    CallToolResult,
    TextContent,
    Tool,
)

# Load .env file if present
load_dotenv()

# OpenRouter configuration
OPENROUTER_API_URL = "https://openrouter.ai/api/v1/chat/completions"
OPENROUTER_MODEL = "perplexity/sonar-pro"

# Create the MCP server
server = Server("search-server")


def get_api_key() -> str:
    """Get OpenRouter API key from environment."""
    api_key = os.getenv("OPENROUTER_API_KEY")
    if not api_key:
        raise ValueError("OPENROUTER_API_KEY environment variable is required")
    return api_key


async def search_web(query: str) -> dict[str, Any]:
    """
    Perform web search using OpenRouter Perplexity Sonar Pro.
    
    Args:
        query: The search query string
        
    Returns:
        Dictionary containing search results with answer and citations
    """
    api_key = get_api_key()
    
    async with httpx.AsyncClient(timeout=60.0) as client:
        response = await client.post(
            OPENROUTER_API_URL,
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "HTTP-Referer": "https://github.com/mcp-proxy",
                "X-Title": "mcp-proxy search server",
            },
            json={
                "model": OPENROUTER_MODEL,
                "messages": [
                    {
                        "role": "system",
                        "content": (
                            "You are a helpful search assistant. Provide accurate, "
                            "up-to-date information with citations when available. "
                            "Format your response clearly and cite your sources."
                        ),
                    },
                    {
                        "role": "user",
                        "content": query,
                    },
                ],
            },
        )
        response.raise_for_status()
        data = response.json()
    
    # Extract the response content
    choices = data.get("choices", [])
    if not choices:
        return {
            "success": False,
            "error": "No response from search model",
            "raw_response": data,
        }
    
    message = choices[0].get("message", {})
    content = message.get("content", "")
    
    # Build result with available metadata
    result: dict[str, Any] = {
        "success": True,
        "query": query,
        "answer": content,
        "model": data.get("model", OPENROUTER_MODEL),
    }
    
    # Include usage info if available
    if "usage" in data:
        result["usage"] = data["usage"]
    
    # Include citations if available (Perplexity models may include these)
    if "citations" in data:
        result["citations"] = data["citations"]
    
    return result


@server.list_tools()
async def list_tools() -> list[Tool]:
    """List available tools."""
    return [
        Tool(
            name="web_search",
            description=(
                "Search the web for current information using Perplexity Sonar Pro. "
                "Returns answers with citations from web sources. "
                "Use for questions requiring up-to-date information, news, facts, "
                "or any topic that benefits from web search."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "The search query or question to search for",
                    },
                },
                "required": ["query"],
            },
        ),
    ]


@server.call_tool()
async def call_tool(name: str, arguments: dict[str, Any]) -> CallToolResult:
    """Handle tool calls."""
    if name != "web_search":
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps({"error": f"Unknown tool: {name}"}),
                )
            ],
            isError=True,
        )
    
    query = arguments.get("query")
    if not query:
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps({"error": "Missing required parameter: query"}),
                )
            ],
            isError=True,
        )
    
    try:
        result = await search_web(query)
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps(result, ensure_ascii=False, indent=2),
                )
            ],
        )
    except httpx.HTTPStatusError as e:
        error_detail = str(e)
        try:
            error_body = e.response.json()
            error_detail = json.dumps(error_body, ensure_ascii=False)
        except Exception:
            error_detail = e.response.text or str(e)
        
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps({
                        "error": "HTTP error from OpenRouter",
                        "status_code": e.response.status_code,
                        "detail": error_detail,
                    }, ensure_ascii=False),
                )
            ],
            isError=True,
        )
    except ValueError as e:
        # Missing API key
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps({
                        "error": "Configuration error",
                        "detail": str(e),
                    }),
                )
            ],
            isError=True,
        )
    except Exception as e:
        return CallToolResult(
            content=[
                TextContent(
                    type="text",
                    text=json.dumps({
                        "error": "Unexpected error during search",
                        "detail": str(e),
                    }),
                )
            ],
            isError=True,
        )


async def run_server():
    """Run the MCP server."""
    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options(),
        )


def main():
    """Entry point for the server."""
    import asyncio
    
    try:
        asyncio.run(run_server())
    except KeyboardInterrupt:
        sys.exit(0)


if __name__ == "__main__":
    main()
