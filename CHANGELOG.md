# Changelog
## v1.0.0 - 2026-11-14
### <!-- 0 -->🚀 Features
- **cli:** Add root command registry
- Add central skill store
- Manage skill links safely
- Remove only managed skills
- **config:** Define configuration model and init example config
- **cli:** Resolve target skill directories from shared flags (SMC-5)
- **source:** Resolve skill sources and obtain repository workspaces
- **source:** Resolve configured skill paths
- **SMC-9:** Implement apply orchestration and confirmation flow
- **cli:** Add install and uninstall commands (SMC-10)
- **cli:** Add operation summaries to apply, install, and uninstall (SMC-11)
- **ASM-3:** Show live per-repo and per-skill status during apply
- **ASM-3:** Add live progress for skill operations
- **SMC-4:** Clarify operation report sections and add overwrite mode
- Generate Homebrew formula from release assets
- Add release scripts

### <!-- 1 -->🐛 Bug Fixes
- Preserve physical skill store copies
- **source:** Validate skill names against absolute roots
- **cli:** Align skill dirs home expansion and add before/after operation logs
- **SMC-5:** Correct skill installation and removal state
- **SMC-4:** Harden operation reports and overwrite paths
- **SMC-4:** Refine operation reporting and overwrite handling
- **SMC-6:** Correct direct skill selection
- Correct release configuration and metadata

### <!-- 10 -->💼 Other
- Print apply plan before confirmation
- Add cross-platform release binaries

### <!-- 2 -->🚜 Refactor
- Handle unchecked errors and remove dead code to satisfy linters

### <!-- 6 -->🧪 Testing
- **cli:** Cover direct command flags, config fallback, and uninstall safety (SMC-10)

### <!-- 7 -->⚙️ Miscellaneous Tasks
- Add project configuration and release workflow
- Initial commit
- Uninstall openspec
- Rename skill-manager to skill-manager-cli
- **SMC-7:** Replace CLAUDE guidance with AGENTS
- Update .taskfiles/task_release.yaml

