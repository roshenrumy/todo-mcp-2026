IMAGE ?= todo-mcp:dev17
NAMESPACE ?= mcp-todo-demo

.DEFAULT_GOAL := help

.PHONY: help setup server up port-forward inspect inspect-reference seed test reset down

help:
	@echo 'Todo MCP commands:'
	@echo '  make server            Run the MCP server locally on http://localhost:8080/mcp'
	@echo '  make up                Start Minikube and deploy the demo stack'
	@echo '  make port-forward      Forward the deployed MCP server to localhost:8080'
	@echo '  make inspect           Start MCPJam Inspector against the deployed server'
	@echo '  make inspect-reference Start the reference MCP Inspector'
	@echo '  make seed              Recreate the demo seed data job'
	@echo '  make test              Run Go tests'
	@echo '  make reset             Delete the demo Kubernetes namespace'
	@echo '  make down              Stop Minikube'

setup:
	minikube start
server:
	DATABASE_PATH=$(CURDIR)/.local/todo.db REQUEST_STATE_SECRET=local-demo-secret go run ./cmd/todo-mcp server
up: setup
	eval $$(minikube docker-env) && docker build -t $(IMAGE) .
	kubectl apply -f deploy/k8s
	kubectl -n $(NAMESPACE) wait --for=condition=complete job/todo-migrate --timeout=90s
	kubectl -n $(NAMESPACE) wait --for=condition=complete job/todo-seed --timeout=90s
	kubectl -n $(NAMESPACE) rollout status deployment/todo-mcp
	kubectl -n $(NAMESPACE) rollout status deployment/todo-import-worker
port-forward:
	kubectl -n $(NAMESPACE) port-forward service/todo-mcp 8080:8080
inspect: port-forward
	@echo 'Connect MCPJam Inspector to http://localhost:8080/mcp in a second terminal.'
	npx @mcpjam/inspector@latest
inspect-reference: port-forward
	npx @modelcontextprotocol/inspector@latest
seed:
	kubectl -n $(NAMESPACE) delete job todo-seed --ignore-not-found
	kubectl apply -f deploy/k8s/seed.yaml
test:
	go test ./...
reset:
	kubectl delete namespace $(NAMESPACE) --ignore-not-found
down:
	minikube stop
