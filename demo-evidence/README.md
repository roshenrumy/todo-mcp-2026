# Todo MCP article evidence pack

This folder is arranged in the order used by the architecture article. Files
under `inspector/` are ready-to-paste MCP request payloads. Capture the response
in MCPJam **Raw** or **Trace**, then save a screenshot beside the corresponding
request using the filename shown below.

| Article evidence | Source | Save as |
| --- | --- | --- |
| `list_todos` stateless request | Inspector → Raw | `inspector/01-list-todos-request.png` |
| discovery cache response | code snippet | `code/02-discovery-cache.go.txt` |
| `import_todos` request | Inspector → Raw | `inspector/03-import-request.png` |
| Task creation response | Inspector → Raw | `inspector/04-task-created.png` |
| Task poll request | Inspector → Raw | `inspector/05-task-poll-request.png` |
| Task progress response | Inspector → Raw | `inspector/06-task-progress.png` |
| MRTR `input_required` | Inspector → Raw | `inspector/07-mrtr-input-required.png` |
| decoded requestState | code snippet | `code/08-request-state-payload.json` |
| MRTR retry request | Inspector → Raw | `inspector/09-mrtr-retry-request.png` |
| `plan_my_day` orchestration | Playground | `playground/10-plan-my-day-orchestration.png` |

## Capture sequence

1. Run `make up`, then in another terminal run `make port-forward`.
2. Connect MCPJam to `http://localhost:8080/mcp`.
3. Use the JSON payloads in `inspector/` as the corresponding MCP calls.
4. For the MRTR retry, copy `requestState` from the `input_required` response
   into `09-mrtr-retry-request.json` before sending it.
5. For the task poll/progress captures, replace `REPLACE_WITH_TASK_ID` with the
   task identifier returned by `import_todos`. Poll during the artificial delay.
6. Capture the full `plan_my_day` Playground conversation: list call, proposed
   plan, approval, and single `bulk_update_todos` call.

The `X-Demo-Instance` header should be visible in raw HTTP evidence whenever
possible; it is the stateless load-balancing demonstration.
