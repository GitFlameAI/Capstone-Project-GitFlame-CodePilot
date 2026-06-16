# Architecture Overview

GitFlame CodePilot is designed as an external integration service.

```text
GitFlame -> Backend -> ML Service
        -> Database
        -> Git workflow payloads
```

The team does not require direct access to the internal GitFlame codebase for Sprint 1.

See [Database Schema](./database-schema.md) for the initial Sprint 1 database ER diagram and bilingual description.

