# Contributing to SprinGo Framework 🚀

First off, thank you for considering contributing to **SprinGo**! It's people like you that make open-source software such a fantastic tool for everyone.

---

## 🛠️ How Can I Contribute?

### 1. Reporting Bugs 🐛
- Check if the bug has already been reported under [GitHub Issues](https://github.com/NeftaliAcosta/springo/issues).
- If not, open a new Issue using a clear title and description including:
  - Go version (`go version`)
  - Operating System
  - Steps to reproduce the error

### 2. Suggesting Enhancements 💡
- Check [GitHub Discussions](https://github.com/NeftaliAcosta/springo/discussions) or Issues to see if the feature is already being discussed.
- Open a new discussion to pitch your idea before writing code for major changes.

### 3. Pull Requests (PRs) 🔀
1. Fork the repository and create your branch from `main`:
   ```bash
   git checkout -b feat/my-new-feature
   ```
2. Make sure code passes formatting, tests, and linting:
   ```bash
   # In root module:
   go fmt ./...
   go vet ./...
   go test -race ./...

   # In demo-api/ module:
   cd demo-api
   go fmt ./...
   go vet ./...
   go test -race ./...
   ```
3. Commit your changes following [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat: add new CLI generator for events`
   - `fix: resolve race condition in EventBus`
   - `docs: update Actuator documentation`
4. Push to your fork and submit a Pull Request to `main`.

---

## 🏗️ Local Development Setup

To test framework changes locally using the `demo-api`:

```bash
git clone https://github.com/NeftaliAcosta/springo.git
cd springo/demo-api
go run cmd/app/main.go
```

The `demo-api/go.mod` uses a local `replace` directive pointing to `../` so any edits to `framework/` take effect immediately.

---

## 📜 Code of Conduct

Please be respectful, open-minded, and constructive in all discussions, issues, and code reviews. Welcome aboard! 💪
