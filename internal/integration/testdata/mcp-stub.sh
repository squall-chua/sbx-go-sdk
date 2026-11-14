#!/bin/sh
# A minimal stdio MCP server for the integration tests: it answers initialize
# and tools/list (with no tools) and nothing else. Requests with no numeric
# id, such as notifications/initialized, get no reply.
while IFS= read -r line; do
	id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
	[ -n "$id" ] || continue
	case "$line" in
	*'"method":"initialize"'*)
		result='{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"sbx-go-sdk-stub","version":"0"}}' ;;
	*'"method":"tools/list"'*) result='{"tools":[]}' ;;
	*) result='{}' ;;
	esac
	printf '{"jsonrpc":"2.0","id":%s,"result":%s}\n' "$id" "$result"
done
