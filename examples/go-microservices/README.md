# Go microservices sample

This standalone module is the deterministic target repository for later thesis
experiments. It models three small service boundaries and deliberately uses
interfaces, hand-written mocks, sentinel errors, and table-friendly tests.

Run it with:

```bash
go test ./...
```

Planned change scenarios include duplicate-email validation, maximum order
quantity, stock validation, repository error handling, and boundary conditions.

