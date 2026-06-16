// Demo data used to make the page look like a real GitFlame repository view.
// In a real integration GitFlame would supply this; here it is static so the
// demo runs without any backend.

export const demoRepo = {
  id: 'demo-repo',
  owner: 'tiroro-20-10',
  name: 'test',
  defaultBranch: 'main',
  webUrl: 'https://gitflametest.ru/tiroro-20-10/test',
  description: 'AI integration sandbox repository for GitFlame CodePilot.',
  branches: ['main', 'develop', 'ai/experiments'],
  language: 'Go',
  stars: 12,
  forks: 3,
  lastCommit: {
    message: 'Add issue workflow endpoints and mock Git workflow',
    author: 'artur',
    hash: 'a1b2c3d',
    when: '2 days ago',
  },
}

// File listing for the Code tab (icon: 'dir' | 'file').
export const demoFiles = [
  { type: 'dir', name: 'backend', message: 'Go API service and integration contracts', when: '2 days ago' },
  { type: 'dir', name: 'frontend', message: 'Vue demo UI (Work with AI + recommendations)', when: 'just now' },
  { type: 'dir', name: 'ml_service', message: 'Open-source model integration and mock endpoints', when: '4 days ago' },
  { type: 'dir', name: 'docs', message: 'Architecture, API contracts and report sections', when: '3 days ago' },
  { type: 'dir', name: 'infra', message: 'Docker and deployment notes', when: '5 days ago' },
  { type: 'file', name: '.ai.yml', message: 'AI behaviour configuration for this repository', when: '1 day ago' },
  { type: 'file', name: 'docker-compose.yml', message: 'Backend, ML service and database services', when: '5 days ago' },
  { type: 'file', name: 'README.md', message: 'Project overview and quick start', when: '2 days ago' },
]

// A valid draft .yml matching the Sprint 1 spec. Used as the starting point in
// the "Work with AI" configuration form.
export const defaultYaml = `version: 1
repository:
  default_branch: main
  target_branch_prefix: ai/
analysis:
  enabled: true
  include:
    - src/**
    - internal/**
  exclude:
    - node_modules/**
    - dist/**
    - build/**
    - .git/**
code_generation:
  enabled: true
  require_user_approval: true
  reviewer_policy: issue_author
  allowed_actions:
    approve_command: "/approve"
    correct_command: "/correct"
    reject_command: "/reject"
recommendations:
  enabled: true
  severity_threshold: low
  categories:
    - code_duplication
    - security
    - maintainability
    - performance
    - architecture
rag:
  max_files: 20
  max_file_size_kb: 120
  context_strategy: issue_relevant_files
storage:
  recommendation_ttl_days: 30
`
