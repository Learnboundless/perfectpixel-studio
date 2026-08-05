---
name: perfectpixel
description: AI 애니메이션 스프라이트 생성. 텍스트 설명 한 줄로 캐릭터 + 동작 애니메이션(걷기·달리기·공격·마법 등 100여 종) + 8방향 스프라이트 세트를 만들고, 게임 엔진용 번들(스프라이트시트 · manifest.json · Aseprite JSON · 상태별 GIF/APNG · 개별 프레임 PNG)로 내보냅니다. Use when the user wants to generate game sprites, character animations, sprite sheets, sprite atlases, or 8-direction sprite sets from a text description.
user-invocable: true
allowed-tools:
  - Bash
  - Read
  - Write
---

# PerfectPixel — AI 애니메이션 스프라이트 생성

설치형 데스크톱 앱과 동일한 생성 파이프라인(프롬프트 → AI 이미지 생성 → 배경 제거
→ 프레임 추출 → 품질 검사 → 보정 재생성 → 픽셀 양자화)을 헤드리스 CLI(`ppgen`)로
구동해, 게임에 바로 쓸 수 있는 스프라이트 번들을 만든다.

사용자가 캐릭터/동작 애니메이션/스프라이트시트/8방향 세트 생성을 요청하면 이 스킬을 쓴다.

전달된 인자: `$ARGUMENTS`

---

## 0. 사전 준비 (매 실행 첫 단계)

`ppgen` 바이너리를 확인/설치한다. 이 `SKILL.md`가 있는 디렉토리(= 스킬 루트)에서
설치 스크립트를 실행한다. 설치 순서는 ① 기존 바이너리 재사용 → ② GitHub Releases에서
OS/arch 맞는 **프리빌트 바이너리 다운로드(Go 불필요)** → ③ 다운로드 실패 시 Go 소스
빌드(또는 공개 저장소 클론) 다. 성공하면 **바이너리 절대경로를 마지막 줄로 출력**한다.

```bash
# SKILL_DIR = 이 SKILL.md 파일이 있는 디렉토리의 절대경로로 치환할 것.
PPGEN="$(bash "$SKILL_DIR/scripts/install.sh" 2>/tmp/ppgen-install.log | tail -1)"
echo "$PPGEN"   # 이후 모든 ppgen 호출에 이 경로를 사용
```

설치 실패(출력이 비어 있거나 실행 불가)면 `/tmp/ppgen-install.log`를 읽고 원인(네트워크
차단 + Go 미설치 등)을 사용자에게 알린다.

## 1. API 키 확인

`ppgen`은 다음 순서로 키를 찾는다: `config.json`(설치형 앱 설정) → 작업 디렉토리/실행
파일 옆의 `.env`·`.env.local` → OS 환경변수 → CLI `-key` 플래그.

지원 프로바이더와 환경변수:

| 프로바이더 | 환경변수 | 기본 모델 |
|---|---|---|
| gemini (기본) | `GEMINI_API_KEY` (또는 `GOOGLE_API_KEY`) | gemini-3-pro-image |
| openai | `OPENAI_API_KEY` | gpt-image-2 |
| openrouter | `OPENROUTER_API_KEY` | google/gemini-3-pro-image-preview |
| fal | `FAL_KEY` (또는 `FAL_API_KEY`) | fal-ai/nano-banana-pro |
| byteplus | `BYTEPLUS_API_KEY` (또는 `ARK_API_KEY`) | seedream-4-0-250828 |
| zetatechs | `ZETATECHS_API_KEY` (또는 `ZETA_API_KEY`) | gpt-image-2 |

키를 어디서도 찾지 못하면, 사용자에게 어떤 프로바이더로 어떤 키를 쓸지 묻거나
`-provider`/`-key`를 직접 지정하게 한다. **키를 추측하거나 지어내지 않는다.**

## 2. 요청 해석 → 플래그 매핑

사용자의 자연어 요청에서 다음을 뽑아 `ppgen` 플래그로 매핑한다.

- 캐릭터 설명 → `-desc "..."` (영어 프롬프트가 품질이 가장 좋다. 한국어 설명이면 핵심을
  영어로 옮겨 전달하되, 사용자에게 보여주는 설명은 원문 유지.)
- 스타일 → `-style` 중 하나: `pixel`(기본), `chibi`, `cartoon`, `retro16`
- 만들 동작들 → `-states "idle,walk,attack"` (쉼표 구분). 동작 이름은 영문 프리셋 키다.
  - 사용 가능한 전체 목록/카테고리는 `"$PPGEN" -dump` 로 확인(`reference/presets.md`에도 요약).
  - "기본 세트만" 같은 모호한 요청 → `-percat 1`(카테고리당 1개) 또는 핵심 4종
    `idle,walk,run,attack` 권장.
  - "전부 다" → `-all` (100여 개, 시간·비용 큼 → 먼저 사용자에게 규모를 알린다).
- 8방향 세트 요청 → `-dirset walk` 처럼 한 동작 지정 (5방향 AI 생성 + 3방향 미러링).
- 출력 폴더 → `-out ./output-dir` (기본 `./perfectpixel-out`).

## 3. 실행

항상 `-json` 으로 실행해 기계 판독 가능한 요약을 받는다. 생성은 상태당 수십 초~수 분
걸리므로 넉넉한 타임아웃으로 백그라운드 실행 후 완료 알림을 기다린다.

```bash
"$PPGEN" \
  -desc "a small knight with silver armor and a blue plume" \
  -style pixel \
  -states "idle,walk,run,attack" \
  -out ./knight-sprites \
  -json
```

비용/속도가 걱정되면 먼저 1개 상태로 시범 실행(`-states idle`)해 품질을 확인하고
사용자 승인 후 전체를 돌린다.

## 4. 결과 해석 및 보고

stdout JSON(`exportSummary`)의 주요 필드:

- `ok`, `outDir`, `provider`, `model`, `style`, `animations`(상태 수), `sheetWidth/Height`
- `files`: 생성된 산출물 목록
- `results[]`: 상태별 `{ found/expected, score(0~100), identity, motion, status, errors }`

`status`가 `frame-mismatch`(프레임 수 불일치)거나 `score`가 낮은(<50) 상태가 있으면
사용자에게 알리고 재생성(`-attempts` 상향, 설명 구체화, 스타일 변경)을 제안한다.

산출 번들(`outDir/`):

```
base.png                      베이스 캐릭터
sprite-sheet.png              스프라이트시트 (행=상태, 열=프레임)
manifest.json                 PerfectPixel 런타임 메타데이터 (schema v2)
sprite-sheet.json             Aseprite 호환 JSON (Phaser/Unity/Godot 임포트)
frames/<state>/frame-NN.png   개별 프레임
gif/<state>.gif               상태별 애니메이션 미리보기
apng/<state>.png              풀 알파 애니메이션
```

사용자에게는 출력 경로, 상태 수/품질 요약, 게임 엔진 임포트 방법(보통 `sprite-sheet.png`
+ `sprite-sheet.json` 한 쌍)을 간단히 안내한다.

---

## 참고

- 프로바이더/모델/환경변수 상세: `reference/providers.md`
- 동작 프리셋 카탈로그: `reference/presets.md` (또는 `"$PPGEN" -dump`)
- 이 스킬은 설치형 앱과 **동일한** Go 파이프라인을 공유한다. 알고리즘 변경은 앱과 함께
  업데이트되며, `install.sh`가 최신 소스로 재빌드한다.
