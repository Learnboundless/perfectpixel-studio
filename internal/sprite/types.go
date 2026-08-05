// Package sprite는 이미지 → 애니메이션 스프라이트 변환 파이프라인을 구현합니다.
package sprite

import "image"

// StateSpec은 하나의 애니메이션 상태(idle, walk 등) 정의입니다.
type StateSpec struct {
	Name   string `json:"name"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   bool   `json:"loop"`
	Action string `json:"action"`
	Facing string `json:"facing"` // 8방향 키 (south 등, 빈 값이면 방향 지시 없음)
}

// ExtractResult는 스트립에서 프레임을 추출한 결과입니다.
type ExtractResult struct {
	Frames        []*image.NRGBA
	FrameMeta     []FrameMeta
	Candidates    []*image.NRGBA
	CandidateMeta []FrameMeta
	Sequence      FrameSequenceMetrics
	Found         int
	Expected      int
	Warnings      []string
}

// FrameMeta records where a generated/extracted frame came from.
type FrameMeta struct {
	Index         int     `json:"index"`
	TimeMs        int     `json:"timeMs,omitempty"`
	SourcePath    string  `json:"sourcePath,omitempty"`
	Quality       float64 `json:"quality,omitempty"`
	Motion        float64 `json:"motion,omitempty"`
	Similarity    float64 `json:"similarity,omitempty"`
	Continuity    float64 `json:"continuity,omitempty"`
	NearDuplicate bool    `json:"nearDuplicate,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	Selected      bool    `json:"selected,omitempty"`
}

// FrameSequenceMetrics explains why a selected animation sequence is usable.
// Values are normalized to [0,1], except DuplicateRate where lower is better.
type FrameSequenceMetrics struct {
	DuplicateRate  float64 `json:"duplicateRate"`
	Continuity     float64 `json:"continuity"`
	MotionCoverage float64 `json:"motionCoverage"`
	LoopClosure    float64 `json:"loopClosure,omitempty"`
	ActiveStartMs  int     `json:"activeStartMs,omitempty"`
	ActiveEndMs    int     `json:"activeEndMs,omitempty"`
}

// StateFrames는 아틀라스 합성에 들어가는 상태별 최종 프레임입니다.
type StateFrames struct {
	Spec   StateSpec
	Frames []*image.NRGBA
}

// FrameRect는 시트 내 프레임 좌표입니다.
type FrameRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Point는 2D 정수 좌표입니다 (피벗/앵커용).
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// AnimationEntry는 매니페스트의 상태별 항목입니다.
// Rects는 셀 좌표, Trims는 셀 내 콘텐츠 bbox(로컬 좌표), Pivot은 공통 발 앵커입니다.
type AnimationEntry struct {
	Row        int         `json:"row"`
	Rows       int         `json:"rows"`
	Frames     int         `json:"frames"`
	FPS        int         `json:"fps"`
	Loop       bool        `json:"loop"`
	DurationMs int         `json:"durationMs"` // 프레임당 표시 시간
	Pivot      Point       `json:"pivot"`      // 셀 로컬 발 앵커(하단 중심)
	Rects      []FrameRect `json:"rects"`      // 시트 절대 좌표
	Trims      []FrameRect `json:"trims"`      // 셀 로컬 콘텐츠 bbox
}

// Manifest는 런타임용 스프라이트시트 메타데이터입니다 (스키마 v2).
type Manifest struct {
	App        string                    `json:"app"`
	Generator  string                    `json:"generator"`
	Schema     string                    `json:"schema"`
	Version    int                       `json:"version"`
	Character  string                    `json:"character"`
	Sheet      SheetInfo                 `json:"sheet"`
	Animations map[string]AnimationEntry `json:"animations"`
}

// SheetInfo는 시트 이미지 정보입니다.
type SheetInfo struct {
	Image      string `json:"image"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	CellWidth  int    `json:"cellWidth"`
	CellHeight int    `json:"cellHeight"`
	Columns    int    `json:"columns"`
	Rows       int    `json:"rows"`
}
