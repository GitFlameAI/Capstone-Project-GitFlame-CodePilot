# AI Configuration Specification

This file describes the user-facing `.yml` configuration for GitFlame CodePilot.

## Example

```yml
repository:
  default_branch: main

analysis:
  enabled: true
  exclude:
    - node_modules/**
    - dist/**
    - build/**
    - .git/**

recommendations:
  enabled: true
  categories:
    - code_duplication
    - security
    - maintainability
    - performance
    - architecture

storage:
  recommendation_ttl_days: 30
```

## Fields

| Field | Meaning |
|---|---|
| `repository.default_branch` | Main repository branch used as the base for analysis and future generated branches. |
| `analysis.enabled` | Enables or disables AI repository analysis. |
| `analysis.exclude` | Paths or folders that AI must ignore during analysis. |
| `recommendations.enabled` | Enables or disables the recommendation system. |
| `recommendations.categories` | Problem categories the system should focus on when generating recommendations. |
| `storage.recommendation_ttl_days` | Number of days recommendation results should be stored. |
