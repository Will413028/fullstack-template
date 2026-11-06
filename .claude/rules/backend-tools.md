---
description: Go tool dependencies module
paths:
  - "backend/tools/go.mod"
  - "backend/tools/go.sum"
---

# Backend tools

- Tidy `backend/tools/go.mod` by running `go mod tidy` inside `backend/tools/`; remove a tool with `go get -modfile=tools/go.mod -tool <path>@none`.
