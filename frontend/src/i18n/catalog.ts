import { Lang } from ".";

// 백엔드(presets.go / direction.go)가 보내는 한국어 라벨을 프론트에서 다국어로 치환한다.
// 프리셋은 안정적인 영문 name을 키로, 카테고리는 백엔드 한국어 문자열을 키로 사용한다.

type L = Record<Lang, string>;

// 100개 프리셋 라벨 (name → 4개 언어)
export const PRESET_LABELS: Record<string, L> = {
  idle: { en: "Idle", es: "Reposo", ko: "대기", zh: "待机" },
  "idle-combat": { en: "Combat Idle", es: "Reposo de combate", ko: "전투 대기", zh: "战斗待机" },
  walk: { en: "Walk", es: "Caminar", ko: "걷기", zh: "行走" },
  run: { en: "Run", es: "Correr", ko: "달리기", zh: "奔跑" },
  sprint: { en: "Sprint", es: "Esprintar", ko: "전력 질주", zh: "冲刺" },
  jump: { en: "Jump", es: "Saltar", ko: "점프", zh: "跳跃" },
  fall: { en: "Fall", es: "Caer", ko: "낙하", zh: "坠落" },
  land: { en: "Land", es: "Aterrizar", ko: "착지", zh: "落地" },
  crouch: { en: "Crouch", es: "Agacharse", ko: "웅크리기", zh: "蹲下" },
  crawl: { en: "Crawl", es: "Gatear", ko: "기어가기", zh: "爬行" },
  climb: { en: "Climb", es: "Trepar", ko: "오르기", zh: "攀爬" },
  swim: { en: "Swim", es: "Nadar", ko: "수영", zh: "游泳" },
  dash: { en: "Dash", es: "Embestida", ko: "대시", zh: "突进" },
  roll: { en: "Roll", es: "Rodar", ko: "구르기", zh: "翻滚" },
  slide: { en: "Slide", es: "Deslizarse", ko: "슬라이딩", zh: "滑行" },
  sit: { en: "Sit", es: "Sentarse", ko: "앉기", zh: "坐下" },
  sleep: { en: "Sleep", es: "Dormir", ko: "잠자기", zh: "睡觉" },
  turn: { en: "Turn", es: "Girar", ko: "돌아서기", zh: "转身" },
  attack: { en: "Attack", es: "Atacar", ko: "공격", zh: "攻击" },
  "attack-heavy": { en: "Heavy Attack", es: "Ataque fuerte", ko: "강공격", zh: "重攻击" },
  combo: { en: "Combo", es: "Combo", ko: "연속 공격", zh: "连击" },
  slash: { en: "Slash", es: "Tajo", ko: "베기", zh: "挥砍" },
  stab: { en: "Stab", es: "Estocada", ko: "찌르기", zh: "突刺" },
  punch: { en: "Punch", es: "Puñetazo", ko: "주먹", zh: "出拳" },
  kick: { en: "Kick", es: "Patada", ko: "발차기", zh: "踢击" },
  uppercut: { en: "Uppercut", es: "Gancho", ko: "어퍼컷", zh: "上勾拳" },
  block: { en: "Block", es: "Bloquear", ko: "막기", zh: "格挡" },
  parry: { en: "Parry", es: "Parada", ko: "패링", zh: "招架" },
  dodge: { en: "Dodge", es: "Esquivar", ko: "회피", zh: "闪避" },
  backstep: { en: "Backstep", es: "Retroceso", ko: "백스텝", zh: "后撤步" },
  shoot: { en: "Shoot", es: "Disparar", ko: "사격", zh: "射击" },
  reload: { en: "Reload", es: "Recargar", ko: "재장전", zh: "装填" },
  aim: { en: "Aim", es: "Apuntar", ko: "조준", zh: "瞄准" },
  throw: { en: "Throw", es: "Lanzar", ko: "던지기", zh: "投掷" },
  "charge-attack": { en: "Charge Attack", es: "Ataque cargado", ko: "차지 공격", zh: "蓄力攻击" },
  "spin-attack": { en: "Spin Attack", es: "Ataque giratorio", ko: "회전 공격", zh: "旋转攻击" },
  "guard-break": { en: "Guard Break", es: "Rompe guardia", ko: "가드 브레이크", zh: "破防" },
  counter: { en: "Counter", es: "Contraataque", ko: "반격", zh: "反击" },
  taunt: { en: "Taunt", es: "Provocar", ko: "도발", zh: "嘲讽" },
  "draw-weapon": { en: "Draw Weapon", es: "Desenfundar", ko: "무기 뽑기", zh: "拔武器" },
  cast: { en: "Cast", es: "Lanzar hechizo", ko: "시전", zh: "施法" },
  "cast-fire": { en: "Fire Cast", es: "Hechizo de fuego", ko: "화염 시전", zh: "火焰施法" },
  "cast-ice": { en: "Ice Cast", es: "Hechizo de hielo", ko: "빙결 시전", zh: "冰冻施法" },
  "cast-lightning": { en: "Lightning Cast", es: "Hechizo de rayo", ko: "번개 시전", zh: "闪电施法" },
  "cast-heal": { en: "Heal Cast", es: "Hechizo de cura", ko: "치유 시전", zh: "治疗施法" },
  summon: { en: "Summon", es: "Invocar", ko: "소환", zh: "召唤" },
  channel: { en: "Channel", es: "Canalizar", ko: "집중", zh: "引导" },
  buff: { en: "Buff", es: "Potenciar", ko: "강화", zh: "增益" },
  "shield-up": { en: "Shield Up", es: "Escudo", ko: "보호막", zh: "护盾" },
  teleport: { en: "Teleport", es: "Teletransporte", ko: "순간이동", zh: "传送" },
  transform: { en: "Transform", es: "Transformar", ko: "변신", zh: "变身" },
  "power-up": { en: "Power Up", es: "Sobrecarga", ko: "파워업", zh: "强化" },
  meditate: { en: "Meditate", es: "Meditar", ko: "명상", zh: "冥想" },
  explode: { en: "Explode", es: "Explotar", ko: "폭발", zh: "爆发" },
  hurt: { en: "Hurt", es: "Daño", ko: "피격", zh: "受击" },
  "hurt-heavy": { en: "Heavy Hurt", es: "Daño fuerte", ko: "강피격", zh: "重受击" },
  knockback: { en: "Knockback", es: "Empuje", ko: "넉백", zh: "击退" },
  knockdown: { en: "Knockdown", es: "Derribo", ko: "넘어짐", zh: "击倒" },
  "get-up": { en: "Get Up", es: "Levantarse", ko: "일어서기", zh: "起身" },
  stun: { en: "Stun", es: "Aturdir", ko: "기절", zh: "眩晕" },
  dizzy: { en: "Dizzy", es: "Mareo", ko: "어지러움", zh: "头晕" },
  frozen: { en: "Frozen", es: "Congelado", ko: "빙결", zh: "冰冻" },
  burning: { en: "Burning", es: "Ardiendo", ko: "화상", zh: "燃烧" },
  poisoned: { en: "Poisoned", es: "Envenenado", ko: "중독", zh: "中毒" },
  stagger: { en: "Stagger", es: "Tambalearse", ko: "비틀거림", zh: "踉跄" },
  death: { en: "Death", es: "Muerte", ko: "사망", zh: "死亡" },
  "death-fall": { en: "Falling Death", es: "Muerte por caída", ko: "추락사", zh: "坠亡" },
  revive: { en: "Revive", es: "Revivir", ko: "부활", zh: "复活" },
  "low-hp": { en: "Low HP", es: "Agonía", ko: "빈사", zh: "濒死" },
  defeat: { en: "Defeat", es: "Derrota", ko: "패배", zh: "战败" },
  wave: { en: "Wave", es: "Saludar", ko: "인사", zh: "挥手" },
  cheer: { en: "Cheer", es: "Animar", ko: "환호", zh: "欢呼" },
  clap: { en: "Clap", es: "Aplaudir", ko: "박수", zh: "鼓掌" },
  bow: { en: "Bow", es: "Reverencia", ko: "절", zh: "鞠躬" },
  nod: { en: "Nod", es: "Asentir", ko: "끄덕임", zh: "点头" },
  "shake-head": { en: "Shake Head", es: "Negar", ko: "도리질", zh: "摇头" },
  laugh: { en: "Laugh", es: "Reír", ko: "웃음", zh: "大笑" },
  cry: { en: "Cry", es: "Llorar", ko: "울음", zh: "哭泣" },
  angry: { en: "Angry", es: "Enfado", ko: "분노", zh: "愤怒" },
  surprised: { en: "Surprised", es: "Sorpresa", ko: "놀람", zh: "惊讶" },
  think: { en: "Think", es: "Pensar", ko: "생각", zh: "思考" },
  point: { en: "Point", es: "Señalar", ko: "가리키기", zh: "指向" },
  salute: { en: "Salute", es: "Saludo militar", ko: "경례", zh: "敬礼" },
  dance: { en: "Dance", es: "Bailar", ko: "춤", zh: "跳舞" },
  victory: { en: "Victory", es: "Victoria", ko: "승리", zh: "胜利" },
  sad: { en: "Sad", es: "Tristeza", ko: "슬픔", zh: "悲伤" },
  scared: { en: "Scared", es: "Miedo", ko: "겁먹음", zh: "害怕" },
  yawn: { en: "Yawn", es: "Bostezar", ko: "하품", zh: "打哈欠" },
  "pick-up": { en: "Pick Up", es: "Recoger", ko: "줍기", zh: "拾取" },
  carry: { en: "Carry", es: "Cargar", ko: "들기", zh: "搬运" },
  push: { en: "Push", es: "Empujar", ko: "밀기", zh: "推" },
  pull: { en: "Pull", es: "Tirar", ko: "당기기", zh: "拉" },
  open: { en: "Open", es: "Abrir", ko: "열기", zh: "打开" },
  eat: { en: "Eat", es: "Comer", ko: "먹기", zh: "进食" },
  drink: { en: "Drink", es: "Beber", ko: "마시기", zh: "喝" },
  read: { en: "Read", es: "Leer", ko: "읽기", zh: "阅读" },
  dig: { en: "Dig", es: "Cavar", ko: "파기", zh: "挖掘" },
  mine: { en: "Mine", es: "Minar", ko: "채굴", zh: "采矿" },
  chop: { en: "Chop", es: "Talar", ko: "베어내기", zh: "砍伐" },
  fish: { en: "Fish", es: "Pescar", ko: "낚시", zh: "钓鱼" },
};

// 카테고리 (백엔드 한국어 문자열 → 4개 언어)
export const CATEGORY_LABELS: Record<string, L> = {
  "기본 동작": { en: "Basics", es: "Básicos", ko: "기본 동작", zh: "基础动作" },
  전투: { en: "Combat", es: "Combate", ko: "전투", zh: "战斗" },
  "마법·스킬": { en: "Magic & Skills", es: "Magia y habilidades", ko: "마법·스킬", zh: "魔法·技能" },
  "피해·상태이상": { en: "Damage & Status", es: "Daño y estados", ko: "피해·상태이상", zh: "受伤·状态" },
  "감정·표현": { en: "Emotion", es: "Emoción", ko: "감정·표현", zh: "情感·表现" },
  상호작용: { en: "Interaction", es: "Interacción", ko: "상호작용", zh: "互动" },
};

// 방향 (key → 4개 언어 라벨)
export const DIRECTION_LABELS: Record<string, L> = {
  "north-west": { en: "¾ Back-L", es: "¾ Atrás-Izq", ko: "¾ 뒤·좌", zh: "¾后·左" },
  north: { en: "Back", es: "Atrás", ko: "뒷면", zh: "背面" },
  "north-east": { en: "¾ Back-R", es: "¾ Atrás-Der", ko: "¾ 뒤·우", zh: "¾后·右" },
  west: { en: "Left", es: "Izquierda", ko: "좌측면", zh: "左侧" },
  east: { en: "Right", es: "Derecha", ko: "우측면", zh: "右侧" },
  "south-west": { en: "¾ Front-L", es: "¾ Frente-Izq", ko: "¾ 앞·좌", zh: "¾前·左" },
  south: { en: "Front", es: "Frente", ko: "정면", zh: "正面" },
  "south-east": { en: "¾ Front-R", es: "¾ Frente-Der", ko: "¾ 앞·우", zh: "¾前·右" },
};

// 아트 스타일 (key → 4개 언어)
export const STYLE_LABELS: Record<string, L> = {
  pixel: { en: "Pixel Art", es: "Pixel Art", ko: "픽셀 아트", zh: "像素风" },
  chibi: { en: "Chibi", es: "Chibi", ko: "치비", zh: "Q版" },
  cartoon: { en: "Cartoon", es: "Cartoon", ko: "카툰", zh: "卡通" },
  retro16: { en: "16-bit Retro", es: "Retro 16-bit", ko: "16비트 레트로", zh: "16位复古" },
  custom: { en: "Custom", es: "Personalizado", ko: "직접 입력", zh: "自定义" },
};

function pick(map: Record<string, L>, key: string, lang: Lang, fallback: string): string {
  const e = map[key];
  if (!e) return fallback;
  return e[lang] || e.en || fallback;
}

export const presetLabel = (name: string, lang: Lang, fallback = name) => pick(PRESET_LABELS, name, lang, fallback);
export const categoryLabel = (cat: string, lang: Lang) => pick(CATEGORY_LABELS, cat, lang, cat);
export const directionName = (key: string, lang: Lang, fallback = key) => pick(DIRECTION_LABELS, key, lang, fallback);
export const styleLabel = (key: string, lang: Lang, fallback = key) => pick(STYLE_LABELS, key, lang, fallback);

// 상태 카드/헤더 표시명을 현재 언어로 구성한다.
// 프리셋 name이면 카탈로그 라벨, custom이면 customWord, 8방향 세트면 방향 접미사를 붙인다.
export function composeStateLabel(
  s: { name: string; label: string; isCustom?: boolean; customLabel?: string; dirBase?: string; facing?: string },
  lang: Lang,
  customWord: string
): string {
  const base = s.dirBase ?? s.name;
  let baseLabel: string;
  if (PRESET_LABELS[base]) baseLabel = presetLabel(base, lang);
  else if (s.isCustom || base.startsWith("custom")) baseLabel = s.customLabel?.trim() || customWord;
  else baseLabel = s.label;
  if (s.dirBase && s.facing) return `${baseLabel}·${directionName(s.facing, lang)}`;
  return baseLabel;
}
