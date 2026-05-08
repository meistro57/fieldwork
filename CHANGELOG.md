# Changelog

All notable changes to this project are documented in this file.

## [Unreleased]

### Added

- Added GitHub Actions CI workflow at `.github/workflows/ci.yml` for Go tests and marker service Python syntax validation.
- Added `.gitignore` entries for local environment artifacts (`.env`, `venv/`, `bin/fieldwork`).
- Added `CHANGELOG.md` for tracking notable project changes.

## [2026-05-08]

### Fixed

- Resolved Marker service dependency conflict by pinning `Pillow==10.4.0` to satisfy `marker-pdf==1.10.0` requirements.

### Docs

- Updated `README.md` with Marker/Pillow compatibility notes for `make service` builds.

## [2026-05-07]

### Added

- Implemented harvest and parse pipeline foundations.
- Initial project scaffolding and first commit.
