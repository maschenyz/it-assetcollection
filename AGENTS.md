@"
# Project Guide

## Overview
This is a web application project.

## Tech Stack
- HTML5
- CSS3
- JavaScript

## Project Structure
- `src/` - Application source code
- `css/` - Stylesheets
- `js/` - JavaScript
- `assets/` - Images and static assets

## Rules
- Read existing code before modifying it.
- Preserve existing functionality.
- Reuse existing functions and components when possible.
- Do not rewrite unrelated files.
- Do not change database/API structure unless explicitly requested.
- Keep changes focused on the requested feature.
- Follow the existing coding style.
- Test changes after implementation.

## Workflow
1. Inspect relevant files first.
2. Understand dependencies and existing behavior.
3. Explain the planned changes briefly.
4. Make the smallest necessary changes.
5. Verify that existing functionality is not broken.

## Important
When a task involves multiple files, inspect the relevant dependencies before making changes.
Do not scan or modify the entire project unnecessarily.
"@ | Set-Content -Encoding UTF8 AGENTS.md

## Development Process

- `process.md` is the project development and progress log.
- Read `process.md` when historical context or recent changes are relevant.
- After completing a significant change, update `process.md`.
- Record what changed, which files were modified, and the reason for the change.
- Do not rewrite historical entries unnecessarily.
- Keep the log concise.