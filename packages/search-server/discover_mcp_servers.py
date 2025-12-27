#!/usr/bin/env python3
"""
MCP Server Discovery Client

Connects to search-server, searches for popular MCP servers,
and generates preset configurations for mcp-proxy.

Usage:
    python discover_mcp_servers.py [--no-search] [--timeout SECONDS]
    
Options:
    --no-search     Skip API search, generate presets from known list only
    --timeout N     Timeout per search query in seconds (default: 30)
"""

import argparse
import asyncio
import json
import os
import re
import sys
from contextlib import asynccontextmanager
from dataclasses import dataclass, field

from dotenv import load_dotenv
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

load_dotenv()

# Default timeout for each search query (seconds)
DEFAULT_TIMEOUT = 30

# Known MCP servers from official list
KNOWN_MCP_SERVERS = [
    ("github", "GitHub API integration"),
    ("filesystem", "Local filesystem operations"),
    ("sqlite", "SQLite database operations"),
    ("postgres", "PostgreSQL database operations"),
    ("puppeteer", "Browser automation"),
    ("brave-search", "Brave Search API"),
    ("google-maps", "Google Maps API"),
    ("slack", "Slack workspace integration"),
    ("memory", "Knowledge graph memory"),
    ("fetch", "HTTP fetch operations"),
    ("git", "Git operations"),
    ("gitlab", "GitLab API integration"),
]

@dataclass
class MCPServerPreset:
    name: str
    description: str
    command: str
    args: list[str]
    env: dict[str, str] = field(default_factory=dict)
    npm_package: str = ""

@asynccontextmanager
async def create_search_session():
    """Connect directly to search-server."""
    server_params = StdioServerParameters(
        command=sys.executable,
        args=["-m", "search_server"],
        env={
            **os.environ,
            "PYTHONPATH": os.path.dirname(os.path.abspath(__file__)),
        },
    )
    
    async with stdio_client(server_params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            yield session

async def search_mcp_servers(session, query: str, timeout: float = DEFAULT_TIMEOUT) -> dict:
    """Search using web_search tool with timeout."""
    try:
        result = await asyncio.wait_for(
            session.call_tool("web_search", {"query": query}),
            timeout=timeout
        )
        if result.isError:
            return {"error": "Search returned error"}
        if result.content:
            try:
                return json.loads(result.content[0].text)
            except:
                return {"answer": result.content[0].text}
    except asyncio.TimeoutError:
        return {"error": f"Search timed out after {timeout}s"}
    except Exception as e:
        return {"error": str(e)}
    return {}

def generate_go_presets(presets: list[MCPServerPreset]) -> str:
    """Generate Go code for presets.go."""
    lines = ['// Additional MCP server presets', '']
    for preset in presets:
        args_str = ', '.join(f'"{a}"' for a in preset.args)
        env_code = ""
        if preset.env:
            env_items = ', '.join(f'"{k}": "{v}"' for k, v in preset.env.items())
            env_code = f'\n\t\t\t\tEnv: map[string]string{{{env_items}}},'
        
        lines.append(f'''			"{preset.name}": {{
				Command: "{preset.command}",
				Args:    []string{{{args_str}}},{env_code}
			}},''')
    return '\n'.join(lines)


def generate_presets_without_search() -> list[MCPServerPreset]:
    """Generate presets from known list without API search."""
    presets = []
    for server_name, description in KNOWN_MCP_SERVERS:
        npm_package = f"@modelcontextprotocol/server-{server_name}"
        presets.append(MCPServerPreset(
            name=server_name,
            description=description,
            command="npx",
            args=["-y", npm_package],
            npm_package=npm_package,
        ))
    return presets


async def discover_with_search(timeout: float) -> list[MCPServerPreset]:
    """Discover presets using search-server API."""
    discovered = []
    
    async with create_search_session() as session:
        for i, (server_name, description) in enumerate(KNOWN_MCP_SERVERS, 1):
            query = f"MCP server {server_name} npm package configuration"
            print(f"[{i}/{len(KNOWN_MCP_SERVERS)}] Searching: {server_name}...", end=" ", flush=True)
            
            result = await search_mcp_servers(session, query, timeout=timeout)
            
            if "error" in result:
                print(f"⚠️  {result['error']}")
            else:
                print("✓")
            
            # Default to official package naming
            npm_package = f"@modelcontextprotocol/server-{server_name}"
            discovered.append(MCPServerPreset(
                name=server_name,
                description=description,
                command="npx",
                args=["-y", npm_package],
                npm_package=npm_package,
            ))
    
    return discovered


def parse_args():
    parser = argparse.ArgumentParser(
        description="Discover MCP servers and generate preset configurations"
    )
    parser.add_argument(
        "--no-search",
        action="store_true",
        help="Skip API search, generate presets from known list only"
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=DEFAULT_TIMEOUT,
        help=f"Timeout per search query in seconds (default: {DEFAULT_TIMEOUT})"
    )
    return parser.parse_args()


async def main():
    args = parse_args()
    
    if args.no_search:
        print("Generating presets without search (--no-search mode)...")
        discovered = generate_presets_without_search()
    else:
        if not os.getenv("OPENROUTER_API_KEY"):
            print("ERROR: OPENROUTER_API_KEY required (or use --no-search)")
            sys.exit(1)
        
        print(f"Starting discovery (timeout: {args.timeout}s per query)...")
        discovered = await discover_with_search(args.timeout)
    
    print(f"\n{'='*60}")
    print(f"Discovered {len(discovered)} MCP server presets")
    print('='*60)
    
    # Generate Go code output
    print("\n--- Go code for presets.go ---\n")
    print(generate_go_presets(discovered))
    
    # Save JSON
    output_file = "discovered_presets.json"
    with open(output_file, "w") as f:
        json.dump({
            "presets": {p.name: {"command": p.command, "args": p.args} for p in discovered}
        }, f, indent=2)
    
    print(f"\n✓ Saved to {output_file}")


if __name__ == "__main__":
    asyncio.run(main())
