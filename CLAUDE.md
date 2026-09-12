@AGENTS.md

## Claude 전용

- 계획 정본: `~/.claude/plans/hazy-splashing-bonbon.md` (v2, Codex 검토 반영). Phase 순서와 게이트는 그 문서를 따른다.
- 커밋 전 `codex-review` 스킬로 교차 검토하고, 지적은 `finding-verifier` → 수정 → `fix-verifier` 순으로 처리한다.
- 무거운 설계 판단(라우팅 결합 알고리즘, 속도 학습 통계)은 `deep-reasoner`에, 기계적 반복 작업은 `fast-worker`에 위임한다.
- Phase 완료 보고는 `docs/` 에 수치·스크린샷을 남긴 뒤에 한다.
