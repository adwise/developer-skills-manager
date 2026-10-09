# Project conventions

This repository contains only the public Go CLI. Keep organization-specific skills, source defaults, manifests, and presentations in separate repositories.

Use the existing Go standard library and installed dependencies. Run `go test ./...` and `go vet ./...` for CLI changes. Test catalog validation against a separate skill repository with `go run ./cmd/skills-manager validate /path/to/catalog`.
