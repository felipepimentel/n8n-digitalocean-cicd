---
layout: default
title: Contributing Guide
description: Guidelines for contributing to the n8n DigitalOcean CI/CD project
---

# Contributing Guide

Thank you for your interest in contributing to the n8n DigitalOcean CI/CD project! This guide will help you get started with contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Documentation Guidelines](#documentation-guidelines)
- [Pull Request Process](#pull-request-process)
- [Release Process](#release-process)

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/version/2/0/code_of_conduct/) Code of Conduct. By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

1. Go 1.21 or later
2. Docker and Docker Compose
3. DigitalOcean account (for testing)
4. Git

### Setting Up Development Environment

1. Fork the repository
   ```bash
   # Clone your fork
   git clone https://github.com/yourusername/n8n-digitalocean-cicd.git
   cd n8n-digitalocean-cicd
   
   # Add upstream remote
   git remote add upstream https://github.com/originalowner/n8n-digitalocean-cicd.git
   ```

2. Install dependencies
   ```bash
   # Initialize Go module
   cd ci
   go mod download
   
   # Install development tools
   go install golang.org/x/tools/cmd/goimports@latest
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

3. Set up pre-commit hooks
   ```bash
   # Install pre-commit
   pip install pre-commit
   pre-commit install
   ```

## Development Workflow

### 1. Branch Naming Convention

- Feature branches: `feature/description`
- Bug fixes: `fix/description`
- Documentation: `docs/description`
- Performance improvements: `perf/description`

Example:
```bash
git checkout -b feature/add-backup-encryption
```

### 2. Commit Message Format

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or modifying tests
- `chore`: Maintenance tasks

Example:
```bash
git commit -m "feat(backup): add encryption to backup process

Implements AES-256 encryption for n8n backups using a secure key.
Closes #123"
```

## Coding Standards

### Go Code Style

1. Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
2. Use `goimports` for formatting:
   ```bash
   goimports -w -local github.com/yourusername/n8n-digitalocean-cicd .
   ```

### Code Organization

```
.
├── ci/
│   ├── main.go           # Main deployment logic
│   ├── ssh/              # SSH client package
│   └── internal/         # Internal packages
├── scripts/
│   ├── monitor.sh        # Monitoring script
│   └── backup.sh         # Backup script
└── docs/                 # Documentation
```

### Error Handling

1. Use custom error types:
   ```go
   type Error struct {
       Op   string
       Err  error
       Code ErrorCode
   }
   
   func (e *Error) Error() string {
       return fmt.Sprintf("%s: %v", e.Op, e.Err)
   }
   ```

2. Wrap errors with context:
   ```go
   if err != nil {
       return fmt.Errorf("failed to create droplet: %w", err)
   }
   ```

### Logging

Use structured logging:
```go
log.WithFields(log.Fields{
    "droplet": dropletName,
    "region":  region,
}).Info("Creating new droplet")
```

## Testing Guidelines

### 1. Unit Tests

```go
func TestCreateDroplet(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        want    *godo.Droplet
        wantErr bool
    }{
        {
            name: "successful creation",
            config: Config{
                dropletName: "test-droplet",
                region:     "nyc1",
            },
            want: &godo.Droplet{
                Name:   "test-droplet",
                Region: &godo.Region{Slug: "nyc1"},
            },
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 2. Integration Tests

```go
func TestIntegrationDeployment(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Test implementation
}
```

### 3. Running Tests

```bash
# Run unit tests
go test ./...

# Run integration tests
go test -tags=integration ./...

# Run tests with coverage
go test -cover ./...
```

## Documentation Guidelines

### 1. Code Documentation

- Document all exported functions, types, and constants
- Include examples for complex functions
- Use complete sentences

Example:
```go
// CreateDroplet creates a new DigitalOcean droplet with the specified configuration.
// It returns the created droplet and any error encountered during the process.
//
// The function will retry the creation process up to 3 times if it encounters
// transient errors.
func CreateDroplet(ctx context.Context, config *Config) (*godo.Droplet, error) {
    // Implementation
}
```

### 2. Markdown Documentation

- Use consistent headers (ATX style)
- Include table of contents for long documents
- Use code blocks with language specification
- Include examples and screenshots where appropriate

## Pull Request Process

1. Create a new branch from `main`
2. Make your changes
3. Run tests and linters:
   ```bash
   # Run all checks
   make check
   ```
4. Update documentation if needed
5. Push changes and create PR
6. Wait for review and address feedback
7. Squash commits if requested
8. Merge after approval

### PR Template

```markdown
## Description
Brief description of the changes

## Type of change
- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Performance improvement

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Linter passes
- [ ] Tested locally
```

## Release Process

1. Version Bumping
   ```bash
   # Create release branch
   git checkout -b release/v1.2.0
   
   # Update version
   echo "1.2.0" > VERSION
   
   # Commit changes
   git commit -am "chore: bump version to 1.2.0"
   ```

2. Release Notes
   - List all notable changes
   - Group by type (features, fixes, etc.)
   - Include upgrade instructions if needed

3. Creating Release
   ```bash
   # Tag release
   git tag -a v1.2.0 -m "Release v1.2.0"
   
   # Push tag
   git push origin v1.2.0
   ```

## Additional Resources

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [GitHub Flow](https://guides.github.com/introduction/flow/)
- [Semantic Versioning](https://semver.org/) 