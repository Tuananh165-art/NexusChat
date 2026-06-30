# NexusChat Engineering Docs

This directory contains generated API documentation and repo-native engineering guidance.

## Core Documents

- [Architecture](architecture.md): service boundaries, data ownership, and runtime dependencies.
- [Clean Code And Design Patterns](clean-code-design-patterns.md): project-wide implementation rules for Go backend and Next.js frontend code.
- [AI Service Plan](ai-service-plan.md): phased architecture and implementation plan for the independent Python AI service.

## Generated API Docs

- `docs/user`
- `docs/match`
- `docs/chat`
- `docs/uploader`

Generated Swagger files should be updated through the existing Make targets instead of hand editing.
