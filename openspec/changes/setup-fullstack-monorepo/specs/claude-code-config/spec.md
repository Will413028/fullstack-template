## ADDED Requirements

### Requirement: Root CLAUDE.md covers fullstack conventions
A root-level `CLAUDE.md` SHALL describe the monorepo structure, how to run services, and conventions for both backend and frontend development.

#### Scenario: CLAUDE.md sections
- **WHEN** Claude Code reads the root `CLAUDE.md`
- **THEN** it SHALL find sections covering: monorepo structure, backend conventions summary, frontend conventions summary, and Docker Compose usage

#### Scenario: Backend conventions referenced
- **WHEN** the root `CLAUDE.md` describes backend conventions
- **THEN** it SHALL reference `backend/CLAUDE.md` for detailed backend architecture

### Requirement: Claude Code skills directory
The `.claude/skills/` directory SHALL contain skills applicable to the fullstack monorepo workflow.

#### Scenario: Skills directory exists
- **WHEN** the setup is complete
- **THEN** `.claude/skills/` SHALL exist at the monorepo root with at least the existing backend skills adapted for monorepo context
