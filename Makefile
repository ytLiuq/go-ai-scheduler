.PHONY: proto
proto:
	PATH=$(shell go env GOPATH)/bin:$$PATH protoc \
		--go_out=. --go_opt=module=github.com/example/go-ai-scheduler \
		--go-grpc_out=. --go-grpc_opt=module=github.com/example/go-ai-scheduler \
		proto/scheduler.proto

.PHONY: build
build: proto
	go build ./cmd/...

.PHONY: run-console
run-console:
	bash ./scripts/run-console.sh

.PHONY: run-full-stack
run-full-stack:
	bash ./scripts/run-full-stack.sh
