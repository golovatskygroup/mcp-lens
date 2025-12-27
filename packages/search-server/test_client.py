#!/usr/bin/env python3
"""
Test client for the search-server MCP server.

Usage:
    python test_client.py [query]

Example:
    python test_client.py "What is the latest Python version?"
"""

import asyncio
import json
import os
import sys
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

# Load environment variables
load_dotenv()


@asynccontextmanager
async def create_client_session():
    """Create an MCP client session connected to the search server."""
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
            # Initialize the connection
            await session.initialize()
            yield session


async def test_list_tools(session: ClientSession) -> None:
    """Test listing available tools."""
    print("=" * 60)
    print("Testing tools/list")
    print("=" * 60)
    
    result = await session.list_tools()
    print(f"Available tools: {len(result.tools)}")
    
    for tool in result.tools:
        print(f"\n  Tool: {tool.name}")
        print(f"  Description: {tool.description}")
        print(f"  Input Schema: {json.dumps(tool.inputSchema, indent=4)}")
    
    print()


async def test_web_search(session: ClientSession, query: str) -> None:
    """Test the web_search tool."""
    print("=" * 60)
    print(f"Testing web_search with query: {query!r}")
    print("=" * 60)
    
    result = await session.call_tool("web_search", {"query": query})
    
    print(f"\nIs error: {result.isError}")
    print("\nResult content:")
    
    for content in result.content:
        if content.type == "text":
            try:
                parsed = json.loads(content.text)
                print(json.dumps(parsed, indent=2, ensure_ascii=False))
            except json.JSONDecodeError:
                print(content.text)
    
    print()


async def main():
    """Run the test client."""
    # Check for API key
    if not os.getenv("OPENROUTER_API_KEY"):
        print("ERROR: OPENROUTER_API_KEY environment variable is required")
        print("Set it in .env file or export it:")
        print("  export OPENROUTER_API_KEY=your-api-key")
        sys.exit(1)
    
    # Get query from command line or use default
    if len(sys.argv) > 1:
        query = " ".join(sys.argv[1:])
    else:
        query = "What is the current weather in Moscow?"
    
    print("\n" + "=" * 60)
    print("MCP Search Server Test Client")
    print("=" * 60 + "\n")
    
    async with create_client_session() as session:
        # Test listing tools
        await test_list_tools(session)
        
        # Test web search
        await test_web_search(session, query)
    
    print("Test completed successfully!")


if __name__ == "__main__":
    asyncio.run(main())
