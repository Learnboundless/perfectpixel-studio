<div align="center">

# MotionPixel

**비디오와 AI 기반 애니메이션 에셋 스튜디오**

MotionPixel은 MIT 라이선스 PerfectPixel 프로젝트를 기반으로 독립 유지보수되는 확장판입니다.

캐릭터 설명 한 줄로 베이스 캐릭터를 만들고, 걷기·달리기·공격·마법 등 100여 가지 동작 애니메이션과
8방향 스프라이트 세트를 자동 생성해 — 게임 엔진이 바로 임포트할 수 있는 형식으로 내보냅니다.

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-v2-DF0000)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)

**[🌐 홈페이지](https://pp.andrew-ai.app/)**

[English](README.md) · **한국어**

<img src="samples/power-up.gif" alt="MotionPixel로 생성한 픽셀아트 파워업 애니메이션" width="280" />

<sub>실제 파이프라인(`gemini-3-pro-image`)으로 처음부터 끝까지 생성한 파워업 애니메이션.</sub>

</div>

---

## 무엇이 어려운 문제인가

그림 한 장 잘 그려 주는 도구는 흔합니다. 하지만 게임 개발자가 원하는 건 다릅니다. 걷기 6프레임이면
정확히 6프레임이어야 하고, 모든 프레임에서 같은 캐릭터로 보여야 하며, 배경은 완전한 알파 투명이어야
하고, 프레임마다 캐릭터 앵커가 흔들리지 않아야 합니다 — 그것도 엔진이 그대로 임포트하는 형식으로요.

AI는 이 조건들을 잘 지키지 못합니다. 프레임 수가 틀리고, 포즈끼리 붙어 버리고, 중간에 머리색이 바뀌는
*identity drift*가 일어나고, chroma key 배경색이 캐릭터 외곽에 번집니다.

**PerfectPixel의 핵심 아이디어: 렌더링은 AI에게 맡기되, 품질·일관성·정밀도는 전부 결정론적
후처리 파이프라인으로 보정한다.** AI 출력이 비결정적이어도 결과물은 항상 일정한 품질로 수렴합니다.
이것이 핵심 기술력(moat)이며, 아래의 전/후 비교는 모두 *실제* 파이프라인 코드를 *실제* AI 출력에
적용해 만든 것입니다.

## 주요 기능

- **텍스트 → 캐릭터**: 설명 한 줄과 스타일(픽셀아트·치비·카툰 등) 선택만으로 베이스 캐릭터를 생성하고
  배경을 자동 제거합니다.
- **100여 가지 동작 프리셋**: 걷기·달리기·점프·공격·마법·감정 표현 등 카테고리별 키워드 카탈로그.
- **8방향 세트**: 5방향은 AI로 생성하고 3개 미러 방향은 좌우 대칭으로 파생해 **생성 비용을 37.5%
  절감**하며 일관된 세트를 만듭니다.
- **자기 보정 품질 루프**: 생성 → 배경 제거 → 프레임 추출 → 품질 검사 → 보정 재생성을 최대 3회 반복해
  프레임 수와 모션 품질을 맞춥니다.
- **진짜 픽셀아트화**: 공유 팔레트 양자화 + 픽셀 그리드 스냅으로 스타일에 맞는 도트 결과물을 만듭니다.
- **게임 엔진 친화 내보내기**: 스프라이트시트 + `manifest.json` + Aseprite 호환 JSON + 상태별
  GIF/APNG + 개별 프레임 PNG를 한 번에 출력합니다.
- **멀티 프로바이더**: Gemini, OpenAI, OpenRouter, fal.ai, BytePlus, ZetaTechs 중 원하는 백엔드를 선택합니다.
- **세션 저장/복원 · 갤러리**: 작업 상태를 디스크에 보관하고, 생성 결과를 갤러리에 자동 보관합니다.

## 샘플 출력

<div align="center">

| 걷기 | 대기 | 환호 | 파워업 |
|------|------|------|--------|
| <img src="samples/walk.gif" width="100" /> | <img src="samples/idle.gif" width="100" /> | <img src="samples/cheer.gif" width="100" /> | <img src="samples/power-up.gif" width="100" /> |
| 춤 | 승리 | 방어 | 체력저하 |
| <img src="samples/dance1.gif" width="100" /> | <img src="samples/victory-north.gif" width="100" /> | <img src="samples/block-east.gif" width="100" /> | <img src="samples/low-hp.gif" width="100" /> |

</div>

## 동작 원리

한 상태(애니메이션)를 생성할 때 백엔드(Go)에서 다음 파이프라인이 실행됩니다.

```
설명 + 스타일 + 동작 프리셋
        │
        ▼
  프롬프트 빌드 ──► AI 이미지 생성 (가로 스트립)
        │
        ▼
  배경 감지 & 제거 ──► 프레임 추출 (셀 단위 분할)
        │
        ▼
  품질 검사 (프레임 수 / 정체성 drift / 모션 유무)
        │
        ├─ 통과 ──────────────► 픽셀 양자화 ──► 완료
        │
        └─ 미달 ──► 보정 피드백 생성 ──► 재생성 (최대 3회)
```

단순 재시도와 다른 두 가지:

- **best-candidate 채점** — 매 attempt를 `score = Found*100 − errors*10` 으로 평가해 최선 후보를
  보관합니다. 완벽하면 즉시 반환하고, 3회를 다 돌려도 완벽하지 않으면 보관해 둔 best를 반환합니다
  (절대 빈손을 주지 않음). API 오류·cancel은 즉시 중단합니다.
- **측정 기반 retry hint** — inspect에서 검출한 결함을 정밀한 영문 교정 지시로 변환해 다음 prompt에
  주입합니다(예: *"직전 결과는 7개 포즈로 읽혔지만 정확히 6개가 필요하다. canvas를 6개 균등 column으로
  나눠라…"*). 사용자 feedback과 합쳐져 **결함을 짚어 가며 수렴하는 closed-loop self-correction**이
  됩니다.

---

## 핵심 기술 — 결정론적 보정

설계 철학을 한 줄로 요약하면 **"휴리스틱이 아니라 신호처리"**입니다. 세 가지 신호처리 축을,
self-diagnostic·self-correcting closed loop가 감쌉니다.

### 1. 배경 제거 — 색차 기반 chroma matting

RGB threshold 대신 색을 **YCbCr**로 변환해, 밝기(luma, Y)는 버리고 색차 성분(Cb, Cr)만으로 배경을
분리합니다. 그늘진 마젠타든 밝은 마젠타든 같은 색으로 인식하고, luma는 보존하고 chrominance만 거칠게
압축하는 JPEG 4:2:0 subsampling에 본질적으로 강건합니다. background key는 **CbCr histogram의
mode(최빈 클러스터)**로 추정하며(평균이 아니라 mode라 gradient·noise에 흔들리지 않음), 캐릭터가 거의
침범하지 않는 네 corner를 우선 샘플링합니다. soft alpha matting은 **Hermite smoothstep**으로 edge
feathering을 부드럽게 하고, **despill**은 key 방향 색 번짐만 깎아내며(캐릭터 고유색 보존),
**4-connectivity flood fill**은 테두리와 단절된 내부 픽셀은 보존하면서 잔여 배경을 지웁니다(캐릭터에
구멍이 뚫리지 않음). opaque·magenta residue 지표가 급증하면 순수 `#FF00FF`로 re-matte하는
**self-diagnostic 마젠타 fallback**까지 둡니다.

![배경 제거 전/후](report-images/01-matting.png)

> **WITHOUT**(단순 RGB threshold): 마젠타 잔여 2,739px + 핑크 halo 8,164px가 외곽에 남습니다.
> **WITH**(YCbCr matting + despill + flood fill): 잔여 2px, halo 3,447px로 깨끗하며 캐릭터
> 본체(불투명 약 45.6만px)는 보존됩니다.

### 2. 프레임 분할 — projection profile + DP 최적 절단

"6프레임 filmstrip"을 요청해도 포즈가 균등하지 않고 팔이 옆 포즈와 닿습니다. PerfectPixel은 OCR의
**projection profile + optimal cut** 기법을 차용합니다. column별 alpha 질량 `P[x] = Σ_y α(x,y)`로
**세로 alpha projection**을 만들면 포즈 사이 gutter가 valley로 나타나고, smoothing 후 content run을
세어 *natural pose count*를 얻습니다. 포즈가 붙어 valley가 사라지면 **동적계획법(DP)**으로
`Σ P[cut] + λ·(width − ideal)²` 비용을 최소화하는 전역 최적 `expected−1`개 cut을 찾습니다. 두 포즈를
한 blob으로 보는 connected-component 방식과 달리, DP cut은 alpha 질량 최소 지점을 찾아 *정확히*
expected개로 가르며 팔다리를 최소한으로만 절단합니다.

![프레임 분할 전/후](report-images/02-segmentation.png)

> 실제 fire-mage *kick* 스트립(9프레임). **WITHOUT**(equal split): 8개 절단선이 모두 캐릭터를
> 가로지릅니다. **WITH**(projection + DP): **캐릭터를 가로지르는 절단선이 0개**, 9개 포즈가 온전히
> 분리됩니다.

### 3. 프레임 정렬 — alpha-weighted centroid

각 포즈를 cell 중앙에 놓을 때 **bounding box 중심**을 쓰면, 팔이나 무기를 한쪽으로 뻗은 포즈는 bbox가
한쪽으로 쏠려 torso가 반대편으로 밀립니다 — 재생 시 캐릭터가 좌우로 jitter합니다. 대신
**alpha-weighted centroid(질량 중심, `cx = Σ(x·α) / Σα`)**를 cell 중앙에 맞추면, 면적이 큰 torso가
centroid를 지배하므로 팔다리를 어떻게 뻗든 torso는 같은 위치에 옵니다. 공통 scale로 캐릭터 크기를
통일하고(downscale만, CatmullRom interpolation), baseline offset으로 점프 궤적을 보존합니다. 게임
sprite에서 가장 중요한 "축이 흔들리지 않는 느낌"을 결정론적으로 보장하는 것입니다.

![centroid 정렬 전/후](report-images/03-centroid.png)

> 실제 fire-mage *dash* 스트립(5프레임). onion-skin 오버레이, 빨간 선이 cell 중심.
> **WITHOUT**(bbox 중심): 캐릭터가 좌우로 어긋남(질량중심 σ = 27.2px).
> **WITH**(alpha-weighted centroid): cell 중앙에 고정(σ = 0.2px, 약 135배 안정).

### 4. 픽셀아트 후처리 — quantization + grid snap

AI가 그린 "픽셀아트"는 진짜 픽셀아트가 아닙니다 — anti-aliasing과 gradient가 섞여 색이 수천 가지인
고해상도 이미지입니다. 전 프레임에서 색을 모아 median-cut으로 **shared palette**를 추출하고(프레임별
양자화는 flicker를 유발), 사람 눈이 녹색에 민감한 점을 반영한 색 거리 `2dr² + 4dg² + 3db²`를 씁니다.
동일색 run length의 최빈값으로 가짜 픽셀의 실제 block 크기를 추정(unfake 기법)한 뒤, 공유 grid에서 각
block을 dominant color로 채우는 **grid snap**을 합니다. identity 검사는 양자화 *전* 원본에 수행해
drift 감지 민감도가 떨어지지 않게 합니다.

![픽셀아트화 전/후](report-images/04-pixelize.png)

> 실제 생성 캐릭터(아래는 4배 확대 crop). **WITHOUT**(raw): 색 7,834가지, 흐릿한 경계.
> **WITH**(shared-palette median-cut + grid snap): 색 12가지, 또렷한 도트 격자.

### Identity & 품질 채점

직교하는 두 축으로 캐릭터 일관성을 검증합니다: **64-bin RGB color histogram**(intersection
similarity, leave-one-out + base 대비로 outlier·batch drift 모두 검출)과 **dHash perceptual
hash**(9×8 grayscale, 구조 민감·색 불변 — histogram이 못 잡는 silhouette 변화 검출). 반대 결함인
**motion presence**(프레임이 너무 *같은* 사실상 정지 화면)도 함께 잽니다. 이 metric들이 0~100점
`ScoreFrames`로 종합됩니다.

```
100 시작
 − (35 + 10·|Found−Expected|)   프레임 수 정확도 (가장 큰 감점)
 − 13·errors  − 3·warnings
 − 12   (2프레임+ 인데 motion < 0.01, 사실상 정지)
 − 10   (dHash identity < 0.55, 구조 붕괴)
→ excellent(≥85) / good(≥70) / fair(≥50) / poor
```

### 보통의 AI 도구 vs PerfectPixel

| 항목 | 보통의 AI 도구 | PerfectPixel |
|------|----------------|--------------|
| 배경 제거 | 고정 RGB chroma threshold | YCbCr 색차 matting + flood fill + morphology + self-diagnostic fallback |
| 프레임 분할 | equal split / connected-component | projection profile + DP global-optimum cut |
| anchor 안정성 | 운에 맡김 | alpha-weighted centroid + manifest foot pivot |
| identity 일관성 | 운에 맡김 | color histogram + dHash structural 2축 + 재생성 loop |
| 품질 측정 | 사람이 눈으로 | 0~100 multi-axis score + headless regression 추적 |
| 압축 강건성 | JPEG noise에 취약 | luma 무시 색차공간, 4:2:0 subsampling에 본질 강건 |

---

## 지원 AI 프로바이더

설정 화면에서 프로바이더를 선택하고 API 키를 입력하면, 유효성 검증 후
`~/Library/Application Support/perfectpixel/config.json`(권한 `0600`)에 저장합니다. 환경변수나 `.env`
파일로도 키를 주입할 수 있습니다.

| 프로바이더 | 기본 모델 | API 키 환경변수 |
|-----------|----------|----------------|
| **Gemini** (기본) | `gemini-3-pro-image` (Nano Banana Pro) | `GEMINI_API_KEY` / `GOOGLE_API_KEY` |
| **OpenAI** | `gpt-image-2` | `OPENAI_API_KEY` |
| **OpenRouter** | `google/gemini-3-pro-image-preview` | `OPENROUTER_API_KEY` |
| **fal.ai** | `fal-ai/nano-banana-pro` | `FAL_KEY` / `FAL_API_KEY` |
| **BytePlus** | `seedream-4-0-250828` (Seedream 4.0) | `BYTEPLUS_API_KEY` / `ARK_API_KEY` |
| **ZetaTechs** | `gpt-image-2` | `ZETATECHS_API_KEY` / `ZETA_API_KEY` |

> 설정 파일의 키가 환경변수보다 우선합니다. 키가 설정된 첫 프로바이더가 자동 활성화됩니다.
> ZetaTechs는 기본 NewAPI 게이트웨이로 `https://ent.zetatechs.com`을 사용합니다. 토큰이 다른
> 게이트웨이에 속하면 `ZETATECHS_BASE_URL` / `ZETA_BASE_URL`로 덮어쓰세요.

## 설치 및 실행

**요구사항**

- [Go](https://go.dev/dl/) 1.25 이상
- [Node.js](https://nodejs.org/) 18 이상 (프론트엔드 빌드)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation):
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- 플랫폼별 의존성은 `wails doctor`로 점검하세요.

**개발 모드 (HMR)**

```bash
git clone https://github.com/gykim80/perfectpixel-studio.git
cd perfectpixel-studio
./dev.sh            # 또는: wails dev
```

`./dev.sh`는 wails CLI를 탐색하고 최초 1회 `frontend/npm install`을 자동 수행합니다.

**프로덕션 빌드**

```bash
wails build         # build/bin/ 에 배포용 앱 생성
```

**API 키 설정**

앱 실행 후 설정 화면에서 키를 입력하거나, 프로젝트 루트에 `.env`를 두세요.

```bash
cp .env.example .env   # 사용할 프로바이더 키만 채우기
```

## 사용법

1. **캐릭터 생성** — 설명을 입력하고 스타일을 골라 베이스 캐릭터를 생성합니다.
2. **동작 추가** — 100여 가지 프리셋(걷기·공격 등)에서 고르거나 직접 입력해 애니메이션 스트립을
   생성합니다.
3. **8방향 세트** *(선택)* — 그리드에서 방향 세트를 생성합니다. 정면 스트립이 다른 방향의 모션
   참조로 쓰입니다.
4. **검토 & 재생성** — 프레임 미리보기와 애니메이션 재생으로 확인하고, 피드백을 더해 다시 생성합니다.
5. **내보내기** — 폴더를 선택하면 게임 엔진에 바로 쓸 수 있는 산출물을 한 번에 저장합니다.

## 내보내기 형식

선택한 폴더 아래 캐릭터 이름 디렉토리에 다음이 생성됩니다.

```
<character>/
├── sprite-sheet.png      # 모든 상태 프레임 아틀라스
├── sprite-sheet.json     # Aseprite 호환 JSON (Phaser/Unity/Godot 임포터)
├── manifest.json         # 상태·프레임·FPS·루프 메타 + foot pivot + 프레임별 trim
├── frames/<state>/       # 상태별 개별 프레임 PNG
├── gif/<state>.gif       # 상태별 미리보기 GIF
└── apng/<state>.png      # 풀 알파 APNG (GIF의 1-bit 투명도 보완)
```

## 프로젝트 구조

```
perfectpixel/
├── main.go            # Wails 앱 진입점 (윈도우/바인딩)
├── app.go             # 프론트엔드에 바인딩되는 핵심 App 메서드 (생성/내보내기/설정)
├── gallery.go         # 갤러리·이미지 입출력 App 메서드
├── internal/
│   ├── config/        # 설정 영속화 + .env/환경변수 폴백
│   ├── gen/           # AI 프로바이더 (gemini, openrouter, fal, byteplus)
│   └── sprite/        # 스프라이트 파이프라인 (chroma · segment · extract · inspect · score · …)
├── cmd/ppvalidate/    # 헤드리스 품질 검증 하니스
├── cmd/ppsamples/     # 전/후 비교 이미지 재생성
├── frontend/          # React + TypeScript + Vite + Tailwind + shadcn/ui
└── build/             # Wails 빌드 리소스 (아이콘/플랫폼 설정)
```

> **`main.go`·`app.go`·`gallery.go`가 루트에 있는 이유**: 셋 다 `package main`입니다. Wails는
> 프론트엔드에 노출되는 `App` 메서드가 루트 main 패키지에 있어야 바인딩(`frontend/wailsjs`)을
> 생성합니다. `gallery.go`는 `app.go`에서 갤러리 메서드를 분리한 의도된 구조입니다.

## 품질 검증 하니스 (ppvalidate)

GUI 없이 실제 생성 파이프라인을 구동해 카테고리/방향별 품질 점수를 수집하는 CLI입니다. 앱과 동일한
3회 보정 루프를 그대로 재현합니다.

```bash
go run ./cmd/ppvalidate -percat 1                    # 카테고리당 1개 키워드
go run ./cmd/ppvalidate -keywords walk,run,attack    # 특정 키워드만
go run ./cmd/ppvalidate -dirset walk                 # walk 8방향 세트
go run ./cmd/ppvalidate -dump                         # 프리셋/방향 카탈로그 JSON 출력
```

활성 프로바이더의 API 키가 필요합니다.

## 비교 이미지 재현 (ppsamples)

위 그림 1~4는 **실제 AI 스프라이트**를 `internal/sprite/`의 **실제 파이프라인 코드**로 처리해 만든
것입니다(합성 아님). 같은 입력을 (a) naive baseline과 (b) 실제 `sprite` 함수에 각각 통과시켜 나란히
합성했으며, 라벨의 수치는 픽셀 통계로 직접 측정한 값입니다.

```bash
go run ./cmd/ppsamples         # report-images/01~04 재생성 + 검증 수치 출력
go run ./cmd/ppsamples scan    # sample/ 스트립을 스캔해 대비 큰 케이스 선정
```

| 그림 | 사용한 실제 코드 | baseline | 결과(전 → 후) |
|------|-----------------|----------|---------------|
| 1 배경 제거 | `sprite.RemoveBackground` | RGB threshold | 마젠타 잔여 2,739 → 2px, halo 8,164 → 3,447px |
| 2 프레임 분할 | `sprite.ExtractFrames` | equal split | 캐릭터 관통 절단선 8/8 → 0/8 |
| 3 프레임 정렬 | `sprite.ExtractFrames` | bbox 중심 | 질량중심 σ 27.2 → 0.2px |
| 4 픽셀아트화 | `sprite.PixelPostProcess` | raw 프레임 | 색 수 7,834 → 12 |

## 테스트

```bash
go test ./...          # 단위 테스트 (live_test.go 의 실 API 테스트는 키 필요)
```

## 기여

이슈와 PR을 환영합니다.

1. 변경 전 `go build ./...` · `go vet ./...` · `go test ./...`가 통과하는지 확인하세요.
2. 코드 스타일: Go는 `gofmt`, 프론트엔드는 TypeScript strict + ESLint/Prettier.
3. 새 동작 프리셋은 `internal/sprite/presets.go`의 `Presets` 슬라이스에 추가하면 프론트엔드와
   백엔드가 함께 인식합니다.

## 라이선스

[MIT](LICENSE) © PerfectPixel contributors
