# Todo MCP 2026 architecture demo

A small, inspectable MCP `2026-07-28` Streamable HTTP server at `POST /mcp`.
It uses the official Go SDK in stateless mode: there is no `Mcp-Session-Id`
dependency, session map, sticky routing, or pod-local pending-operation state.
`X-Demo-Instance` exposes the serving pod so round-robin behavior is visible.

## Run

```sh
make up
make port-forward
# In another terminal:
make inspect
```

Connect MCPJam to `http://localhost:8080/mcp`. `create_todo` deliberately
publishes draft-2020-12 `oneOf`, `$defs`, and `$ref` scheduling variants.
Moving the seeded **Publish MCP article** to Done returns authenticated,
expiring `requestState`; retry it with `choice` set to `move_parent_only`,
`move_with_subtasks`, or `cancel`.

## Async import

Call `import_todos` with items from `fixtures/legacy-todos.json`. It stores the
Task and import job before returning its task handle. Poll `tasks_get`, and use
`tasks_cancel` to request cancellation; the separate worker checks the durable
cancellation flag between items. `IMPORT_DELAY_MS` controls the demo delay.

## Stateless routing

There is no MCP session state, session-to-user mapping, pending-operation map,
or sticky session requirement. Each HTTP response carries `X-Demo-Instance`;
repeated calls through the Kubernetes Service can therefore show alternating
pod hostnames. `Mcp-Method` and `Mcp-Name` request headers are standardized by
the protocol and can support gateway routing/policies. Capability discovery is
safe to cache as public data (a five-minute TTL is appropriate for this demo).

`plan_my_day` is deliberately a host-side planning concern: this server offers
domain tools, not sampling or model-provider calls.

## Storage note

The shared SQLite PVC is intentionally optimized for a single-node Minikube
demo. It is not a production multi-node Kubernetes database architecture.
