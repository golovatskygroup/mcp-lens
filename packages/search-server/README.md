# search-server

MCP server for web search via OpenRouter Perplexity Sonar Pro.

## Overview

This server provides a `web_search` tool that uses OpenRouter's Perplexity Sonar Pro model to search the web and return answers with citations.

## Installation

```bash
cd packages/search-server
pip install -e .
```

Or install dependencies directly:

```bash
pip install -r requirements.txt
```

## Configuration

Set the `OPENROUTER_API_KEY` environment variable:

```bash
export OPENROUTER_API_KEY=your-api-key
```

Or create a `.env` file:

```env
OPENROUTER_API_KEY=your-api-key
```

## Usage

### As standalone server

```bash
# Run as module
python -m search_server

# Or use the entry point (after pip install)
search-server
```

### With mcp-proxy

Add to your `config.yaml`:

```yaml
upstream:
  command: "python"
  args: ["-m", "search_server"]
  env:
    OPENROUTER_API_KEY: "${OPENROUTER_API_KEY}"
```

Or use environment variables:

```bash
export MCP_LENS_UPSTREAM_COMMAND=python
export MCP_LENS_UPSTREAM_ARGS_JSON='["-m", "search_server"]'
export MCP_LENS_UPSTREAM_ENV_JSON='{"OPENROUTER_API_KEY":"'$OPENROUTER_API_KEY'"}'
```

## Tool: web_search

### Description

Search the web for current information using Perplexity Sonar Pro. Returns answers with citations from web sources.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "The search query or question to search for"
    }
  },
  "required": ["query"]
}
```

### Output

```json
{
  "success": true,
  "query": "What is the latest Python version?",
  "answer": "The latest stable version of Python is 3.12.x...",
  "model": "perplexity/sonar-pro",
  "usage": {
    "prompt_tokens": 50,
    "completion_tokens": 200,
    "total_tokens": 250
  }
}
```

## Testing

Run the test client:

```bash
# Default query
python test_client.py

# Custom query
python test_client.py "What is the latest news about AI?"
```

## Development

### Project Structure

```
packages/search-server/
├── pyproject.toml          # Project configuration
├── requirements.txt        # Dependencies
├── search_server/
│   ├── __init__.py
│   └── server.py           # MCP server implementation
├── test_client.py          # Test client
└── README.md
```

### Running locally

```bash
# Install in development mode
pip install -e .

# Run server
python -m search_server

# In another terminal, run test
python test_client.py "Your search query"
```

## API Details

### OpenRouter Configuration

- **API URL**: `https://openrouter.ai/api/v1/chat/completions`
- **Model**: `perplexity/sonar-pro`
- **Timeout**: 60 seconds

### Error Handling

The server handles various error cases:

- Missing API key: Returns configuration error
- HTTP errors: Returns status code and error detail
- Missing query: Returns validation error
- Unexpected errors: Returns error detail

## License

MIT
