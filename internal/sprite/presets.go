package sprite

import "strings"

// PresetInfo는 하나의 상황 키워드(애니메이션 프리셋) 정의입니다.
// Hint는 서버 측 프롬프트 생성에만 쓰이며 json:"-"로 프론트엔드에 노출되지 않습니다.
type PresetInfo struct {
	Name     string `json:"name"`     // 영문 상태명 (export/파일명용)
	Label    string `json:"label"`    // 한국어 표시명
	Category string `json:"category"` // UI 그룹핑용 카테고리 (한국어)
	Action   string `json:"action"`   // 영문 동작 설명 (프롬프트용)
	Frames   int    `json:"frames"`   // 기본 프레임 수
	FPS      int    `json:"fps"`      // 기본 재생 속도
	Loop     bool   `json:"loop"`     // 기본 반복 여부
	Hint     string `json:"-"`        // 모션 가이드 (프롬프트 주입용, 노출 안 함)
}

// Presets는 선택 가능한 100개 상황 키워드 카탈로그입니다.
// 이 슬라이스가 프리셋(프론트) + 모션 힌트(백엔드)의 단일 소스입니다.
var Presets = []PresetInfo{
	// ── 기본 동작 ──
	{"idle", "待机", "基础动作", "subtle breathing idle standing in place", 4, 6, true, "Subtle in-place breathing cycle: gentle chest rise and fall, tiny up-down body shift of a few pixels, occasional blink. Feet stay planted in the same spot in every frame."},
	{"idle-combat", "战斗待机", "基础动作", "ready combat stance, weapon up, weight shifting", 4, 8, true, "Alert combat-ready idle: knees slightly bent, weapon or fists raised, weight shifting subtly side to side, small breathing bob. Feet stay planted; stance never relaxes."},
	{"walk", "行走", "基础动作", "side-view walking cycle facing right", 6, 10, true, "Readable side-view walking cycle: alternating legs with clear contact and passing poses, opposite arm swing, slight body bob. Each frame shows a distinctly different leg position."},
	{"run", "奔跑", "基础动作", "fast side-view running cycle facing right", 6, 12, true, "Fast side-view running cycle: strong forward lean, large leg extension with airborne moments, pumping arms, pronounced body bob. Each frame is a distinct stride phase."},
	{"sprint", "全速冲刺", "基础动作", "all-out sprint, extreme lean and stride", 6, 14, true, "All-out sprint: extreme forward lean, maximal leg extension, both feet airborne at peak, arms pumping hard. Faster, larger strides than a normal run."},
	{"jump", "跳跃", "基础动作", "crouch, take off, airborne peak, land", 5, 10, false, "Jump sequence: crouching anticipation, take-off with body extended upward, airborne peak with legs tucked, landing recovery crouch. Vary the body's vertical position to show the arc."},
	{"fall", "下落", "基础动作", "falling through the air", 4, 10, true, "Falling cycle: body airborne, arms and legs flailing or bracing, slight rotation or wobble, hair and clothes pushed upward by wind. No ground contact in any frame."},
	{"land", "落地", "基础动作", "land from a fall and absorb impact", 4, 12, false, "Landing impact: feet touch down, deep knee bend to absorb shock, body compresses low, then rises back toward standing. Show the compression clearly in the middle frame."},
	{"crouch", "蹲伏", "基础动作", "lower into a compact crouch and hold", 4, 8, false, "Crouching sequence: from standing, bend knees and lower the body progressively into a compact crouch, head tucked slightly. Final frame is fully crouched."},
	{"crawl", "爬行", "基础动作", "crawl forward on hands and knees", 6, 8, true, "Hands-and-knees crawling cycle: alternating arm-and-opposite-leg reaches, low body close to the ground, head up. Each frame a distinct crawl phase."},
	{"climb", "攀爬", "基础动作", "climb up a vertical surface", 6, 8, true, "Vertical climbing cycle: alternating hand-over-hand reaches and matching foot pushes, body pressed close to the surface, upward progress implied. Each frame a distinct reach."},
	{"swim", "游泳", "基础动作", "swimming stroke cycle", 6, 8, true, "Swimming stroke cycle: arms reaching forward and pulling back in alternation, legs kicking, body horizontal. Each frame a distinct stroke phase."},
	{"dash", "突进", "基础动作", "quick burst dash forward", 4, 14, false, "Quick dash burst: explosive crouch-and-push start, body stretched low and forward at peak speed, then a brief settle. Strong horizontal lean throughout."},
	{"roll", "翻滚", "基础动作", "evasive forward roll", 5, 14, false, "Evasive forward roll: tuck into a ball, rotate fully over the shoulder, and rise back to a crouch. Show clear rotation phases across frames."},
	{"slide", "滑铲", "基础动作", "sliding low along the ground", 4, 12, false, "Low slide: drop into a feet-first slide with one leg extended, body leaning back low to the ground, then begin to rise. Body stays low across frames."},
	{"sit", "坐下", "基础动作", "sit down to the ground", 4, 8, false, "Sitting down: bend at knees and hips, lower the body, settle onto the ground in a relaxed seated pose. Final frame clearly seated."},
	{"sleep", "睡眠", "基础动作", "sleeping lying down, gentle breathing", 4, 4, true, "Sleeping cycle: lying down with eyes closed, slow gentle breathing rise and fall, occasional small shift. Very calm, minimal motion."},
	{"turn", "转身", "基础动作", "turn around to face the other way", 4, 10, false, "Turn-around: rotate the body from facing one way to the opposite, weight pivoting on the feet, head leading the turn. Show clear intermediate angles."},

	// ── 전투 ──
	{"attack", "攻击", "战斗", "melee attack with wind-up, strike, recovery", 5, 12, false, "Melee attack: wind-up with body coiled back, powerful strike at full extension, follow-through, recovery to ready stance. The strike frame is the most extreme pose."},
	{"attack-heavy", "重攻击", "战斗", "slow heavy melee attack with big wind-up", 6, 10, false, "Heavy attack: long exaggerated wind-up loading weight back, a slow powerful swing, deep follow-through, slow recovery. Bigger and slower than a normal attack."},
	{"combo", "连击", "战斗", "multi-hit melee combo", 6, 14, false, "Multi-hit combo: a fast sequence of distinct strikes from different angles (e.g. slash, backslash, thrust), each frame a separate hit, ending in a recovery pose."},
	{"slash", "斩击", "战斗", "horizontal sword slash", 5, 14, false, "Sword slash: coil the blade back, sweep it across in a wide horizontal arc at full extension, follow through to the opposite side, recover. Most extreme pose mid-swing."},
	{"stab", "突刺", "战斗", "forward thrust attack", 4, 14, false, "Thrust attack: draw the weapon back close to the body, explosive straight forward lunge with full arm and weapon extension, then retract. Peak frame fully extended forward."},
	{"punch", "拳击", "战斗", "straight punch", 4, 14, false, "Straight punch: cock the fist back at the hip, drive it forward with shoulder rotation to full extension, retract to guard. Peak frame fully extended."},
	{"kick", "踢击", "战斗", "high kick", 5, 14, false, "High kick: plant and chamber the knee, snap the leg out to full extension, hold the impact pose, retract and settle. Peak frame at maximum leg extension."},
	{"uppercut", "上勾拳", "战斗", "rising uppercut punch", 4, 14, false, "Uppercut: dip the body low loading the legs, drive upward exploding the fist up through the target, finish with body extended tall. Peak frame reaching upward."},
	{"block", "格挡", "战斗", "raise guard and hold a defensive block", 3, 10, true, "Defensive block: raise arms or shield to guard, brace with a slight crouch, hold firm with tiny tension shifts. Feet planted, posture steady."},
	{"parry", "招架", "战斗", "deflect an incoming attack", 4, 16, false, "Parry: a sharp deflecting flick of the weapon or arm to one side that knocks an attack away, then snap back to ready. Quick and crisp."},
	{"dodge", "闪避", "战斗", "quick sidestep dodge", 4, 16, false, "Dodge: a fast lean-and-step to one side to evade, body weaving out of the way, then recovering balance. Quick lateral motion."},
	{"backstep", "后撤步", "战斗", "quick hop backward", 4, 14, false, "Backstep: a quick defensive hop backward, light push off the front foot, brief airborne drift, land back in guard. Net backward movement."},
	{"shoot", "射击", "战斗", "fire a ranged weapon", 4, 14, false, "Ranged shot: steady the weapon, fire with a sharp recoil kick pushing the body back, then settle back on target. Show the recoil clearly. No projectile particles separated from the weapon."},
	{"reload", "装填", "战斗", "reload a ranged weapon", 5, 10, false, "Reload sequence: lower the weapon, work the mechanism with the off hand (eject, insert, seat), and raise back to ready. Hands do the distinct work across frames."},
	{"aim", "瞄准", "战斗", "hold a steady aim down sights", 3, 10, true, "Aiming hold: weapon raised and leveled, body steady and braced, only tiny breathing sway. Posture locked, eyes down the sights."},
	{"throw", "投掷", "战斗", "throw an object overhand", 5, 12, false, "Overhand throw: wind the arm back behind the head, whip it forward releasing at full extension, follow through across the body. Peak frame at release."},
	{"charge-attack", "蓄力攻击", "战斗", "charge up then release a powerful attack", 6, 12, false, "Charged attack: a held loading pose gathering power (body coiled, weapon drawn back), then an explosive release strike at full extension, then recovery. Hold the charge for the first frames."},
	{"spin-attack", "旋转攻击", "战斗", "spinning 360 attack", 6, 14, false, "Spin attack: rotate the whole body a full turn while sweeping the weapon around in a wide circle, then settle facing forward. Show distinct rotation angles per frame."},
	{"guard-break", "破防", "战斗", "stagger backward with guard broken", 4, 12, false, "Guard break: the raised guard is smashed open, arms fly apart, body rocks backward off balance, briefly exposed. Recoil reads as defense failing."},
	{"counter", "反击", "战斗", "absorb a hit then counterattack", 5, 14, false, "Counter: a tight defensive flinch, then an immediate sharp counterattack exploding forward at full extension, then recovery. Two-beat defense-into-offense."},
	{"taunt", "挑衅", "战斗", "taunting gesture toward an enemy", 4, 8, true, "Taunt: a confident provoking gesture — beckoning with a hand, chest puffed, head cocked — looping with attitude. Feet planted, upper body expressive."},
	{"draw-weapon", "拔出武器", "战斗", "draw a weapon and enter ready stance", 5, 10, false, "Draw weapon: reach for the weapon, pull it free in a sweeping motion, settle into a ready combat stance. Final frame is the ready pose with weapon up."},

	// ── 마법·스킬 ──
	{"cast", "施法", "魔法与技能", "generic spell casting", 5, 12, false, "Spell casting: arms gather inward in concentration, then thrust forward in a casting pose, followed by recovery. Pose changes only, no floating magical particles."},
	{"cast-fire", "火焰施法", "魔法与技能", "cast a fire spell", 6, 12, false, "Fire spell cast: gather energy at the hands with a coiled stance, then thrust both hands forward releasing the blast. Any flame must be opaque, hard-edged, and touching the hands, not floating particles."},
	{"cast-ice", "冰霜施法", "魔法与技能", "cast an ice spell", 6, 10, false, "Ice spell cast: a slow controlled gathering pose, hands sweeping inward, then a sharp pointed release forward. Cold, precise, deliberate motion."},
	{"cast-lightning", "闪电施法", "魔法与技能", "cast a lightning spell", 5, 14, false, "Lightning cast: a fast raise of the arm overhead charging, then a sharp downward or forward strike releasing the bolt. Quick and snappy. Effects hard-edged and touching the hand only."},
	{"cast-heal", "治疗施法", "魔法与技能", "cast a healing spell on self", 5, 8, false, "Healing cast: bring hands together at the chest in a gentle gathering pose, then open them outward and upward in a soft release, head tilted up. Calm, flowing motion."},
	{"summon", "召唤", "魔法与技能", "summon by raising arms", 5, 10, false, "Summon: crouch and gather low, then rise sweeping both arms upward and outward in a grand calling gesture, finishing tall with arms raised. Build to the peak."},
	{"channel", "引导", "魔法与技能", "channel energy continuously", 4, 8, true, "Channeling loop: a sustained focused pose, hands held out gathering energy, body tense with small pulsing shifts and a slight glow at the hands. Looping concentration."},
	{"buff", "强化", "魔法与技能", "self power-up buff gesture", 4, 10, false, "Buff cast: clench fists and pull them inward to the body in a powering-up motion, body tensing and rising slightly, finishing in a strong braced pose."},
	{"shield-up", "护盾", "魔法与技能", "raise a magical shield barrier", 4, 10, false, "Shield up: sweep one arm forward and out to project a barrier, body braced behind it, then hold. Any barrier shape must be opaque and hard-edged."},
	{"teleport", "瞬移", "魔法与技能", "vanish and reappear", 5, 14, false, "Teleport: body compresses and distorts shrinking away to nothing in the first frames, then reforms and expands back into a solid pose. Use silhouette compression, not particle clouds."},
	{"transform", "变身", "魔法与技能", "dramatic transformation", 6, 10, false, "Transformation: a crouched gathering pose, body tensing and shaking, then bursting upward into a new powered-up stance. Build tension then release to a bold final pose."},
	{"power-up", "蓄能", "魔法与技能", "powering up with surging energy", 5, 10, true, "Power-up loop: braced wide stance, fists clenched, body trembling with effort and energy surging upward, hair and clothes lifting. Looping intensity, feet planted."},
	{"meditate", "冥想", "魔法与技能", "sitting meditation, calm breathing", 4, 4, true, "Meditation loop: seated cross-legged, hands resting on knees, eyes closed, very slow calm breathing rise and fall. Minimal serene motion."},
	{"explode", "爆发", "魔法与技能", "release an explosive burst outward", 5, 16, false, "Explosive release: gather tightly inward, then throw the whole body open releasing a burst outward, then settle. Use the body opening up to imply the blast, effects hard-edged and touching the body."},

	// ── 피해·상태이상 ──
	{"hurt", "受击", "受击与异常", "recoil from being hit", 3, 10, false, "Hit reaction: body recoils backward, head snaps back, brief stagger with arms flailing slightly, then a weakened guard pose. Feet roughly in place."},
	{"hurt-heavy", "重度受击", "受击与异常", "stagger hard from a heavy hit", 4, 10, false, "Heavy hit reaction: the whole body is thrown backward and folds from the impact, head whipping back, nearly losing balance, then a struggling recovery. Bigger than a normal hurt."},
	{"knockback", "击退", "受击与异常", "knocked backward through the air", 4, 12, false, "Knockback: launched backward off the feet from a blow, body airborne and tumbling backward, then a hard skidding stop. Clear backward travel through the air."},
	{"knockdown", "倒地", "受击与异常", "knocked down to the ground", 4, 10, false, "Knockdown: struck and losing footing, body rotates and drops, landing flat on the back or side on the ground. Final frame fully down."},
	{"get-up", "起身", "受击与异常", "get up from the ground", 5, 8, false, "Get up: from lying on the ground, push up with the arms, draw the legs under, rise through a crouch back to standing. Clear upward progression."},
	{"stun", "眩晕", "受击与异常", "stunned and wobbling in place", 4, 8, true, "Stunned loop: dazed slumped posture, head lolling, body swaying off balance, knees buckling slightly. Looping wobble, feet barely holding."},
	{"dizzy", "晕眩", "受击与异常", "dizzy with head spinning", 4, 8, true, "Dizzy loop: head rolling in circles, body wobbling, arms loose and drifting for balance, unfocused. Looping disorientation. No floating star particles."},
	{"frozen", "冻结", "受击与异常", "frozen stiff and trembling", 3, 6, true, "Frozen loop: body locked rigid mid-pose, arms clamped to the sides, only a tiny brittle tremble. Stiff and immobile, shivering slightly."},
	{"burning", "燃烧", "受击与异常", "on fire, flinching from flames", 4, 12, true, "Burning loop: flinching and writhing, patting at the body, hopping in discomfort. Any flame must be opaque, hard-edged, and touching the body, not floating particles."},
	{"poisoned", "中毒", "受击与异常", "sickened by poison, hunched", 4, 6, true, "Poisoned loop: hunched and queasy, clutching the stomach, swaying weakly, head drooping. Looping sickly weakness."},
	{"stagger", "踉跄", "受击与异常", "stumble and barely keep balance", 4, 10, false, "Stagger: lurch off balance, arms windmilling to recover, feet shuffling to catch the body, then steadying. Reads as almost falling."},
	{"death", "死亡", "受击与异常", "stagger, collapse, lie flat on the ground", 5, 8, false, "Defeat sequence: stagger, collapse to the knees, fall further down, finally lying flat on the ground. Final frame clearly lying down."},
	{"death-fall", "坠落死亡", "受击与异常", "fall backward and collapse", 4, 8, false, "Falling death: thrown backward, arms flung out, body arcing back and dropping, landing flat and motionless. Final frame fully down and still."},
	{"revive", "复活", "受击与异常", "rise back to life from the ground", 6, 8, false, "Revive: from lying flat, the body stirs, lifts, and rises through a kneeling pose back to a strong standing stance, head lifting last. Gradual return of strength."},
	{"low-hp", "濒死", "受击与异常", "near death, weak and hunched", 4, 6, true, "Low-HP loop: hunched and exhausted, one hand braced on a knee, heavy labored breathing, slight unsteady sway. Barely standing, looping fatigue."},
	{"defeat", "失败", "受击与异常", "drop to knees in defeat", 4, 8, false, "Defeat: shoulders sag, the body sinks down onto the knees, head bowing low in surrender. Ends kneeling and dejected."},

	// ── 감정·표현 ──
	{"wave", "挥手", "情绪与表现", "friendly hand wave, body still", 4, 8, true, "Friendly greeting: one arm raises and waves side to side across frames while the rest of the body stays still. Hand in clearly different positions each frame. Feet planted."},
	{"cheer", "欢呼", "情绪与表现", "cheer with arms raised", 4, 10, true, "Cheering loop: throw both arms up overhead repeatedly with a small hop or bounce, head up, joyful. Energetic looping celebration."},
	{"clap", "鼓掌", "情绪与表现", "clapping hands", 4, 10, true, "Clapping loop: bring both hands together and apart in front of the chest repeatedly, slight body bounce. Hands clearly open and closed across frames."},
	{"bow", "鞠躬", "情绪与表现", "respectful bow", 4, 8, false, "Bow: from standing, bend forward at the waist into a respectful bow, hold briefly, then rise back up. Show the full forward bend."},
	{"nod", "点头", "情绪与表现", "nodding the head yes", 3, 8, false, "Nod: tip the head down and back up in agreement, small body settle. Head clearly moves down then up. Body otherwise still."},
	{"shake-head", "摇头", "情绪与表现", "shaking the head no", 4, 8, false, "Head shake: turn the head left and right in refusal, shoulders slightly tense. Head clearly rotates side to side. Body otherwise still."},
	{"laugh", "大笑", "情绪与表现", "laughing happily", 4, 8, true, "Laughing loop: head tipped back, shoulders bouncing with laughter, maybe a hand to the belly, big smile. Looping bounce of joy."},
	{"cry", "哭泣", "情绪与表现", "crying sadly", 4, 6, true, "Crying loop: hands toward the face, shoulders shaking with sobs, head bowed, body hunched. Looping sad tremble. Tears optional but must be small and on the face."},
	{"angry", "愤怒", "情绪与表现", "furious, fists clenched", 4, 8, true, "Angry loop: fists clenched, shoulders raised and tense, body trembling with rage, leaning forward, brows down. Looping fury, feet planted."},
	{"surprised", "惊讶", "情绪与表现", "startled and recoiling", 3, 12, false, "Surprise: a sharp startled jolt — body snaps upright and back, arms fly up, head rears, eyes wide. Quick recoil then a frozen shocked pose."},
	{"think", "思考", "情绪与表现", "thinking with hand on chin", 4, 6, true, "Thinking loop: one hand to the chin, head tilted, weight shifting slowly side to side, occasional small head tilt. Pondering, looping subtle motion."},
	{"point", "指向", "情绪与表现", "point forward decisively", 4, 10, false, "Pointing: draw the arm back then thrust it forward extending one finger to point decisively, body leaning into it, then hold. Peak frame fully extended forward."},
	{"salute", "敬礼", "情绪与表现", "military salute", 4, 8, false, "Salute: snap one hand up to the brow in a crisp military salute, body straightening to attention, hold, then lower. Sharp and formal."},
	{"dance", "跳舞", "情绪与表现", "rhythmic dancing", 6, 10, true, "Dancing loop: rhythmic full-body movement — hips and arms swaying, weight shifting foot to foot, head bobbing to a beat. Distinct fun poses per frame, looping smoothly."},
	{"victory", "胜利", "情绪与表现", "victory pose celebration", 4, 8, true, "Victory loop: a triumphant pose — fist pumped or arms raised, chest out, small confident bounce. Looping celebration, proud and energetic."},
	{"sad", "悲伤", "情绪与表现", "sad and downcast", 4, 4, true, "Sad loop: shoulders slumped, head down, arms hanging limp, a slow heavy sway and sigh. Looping melancholy, minimal motion."},
	{"scared", "害怕", "情绪与表现", "frightened and cowering", 4, 8, true, "Scared loop: cowering back, arms raised defensively in front of the face, body trembling and shrinking, knees together. Looping fear."},
	{"yawn", "打哈欠", "情绪与表现", "yawning sleepily", 4, 6, false, "Yawn: a big stretch with arms rising and head tilting back, mouth wide in a yawn, then arms lower and shoulders settle. One clear stretch-and-relax."},

	// ── 상호작용 ──
	{"pick-up", "拾取", "互动", "bend down and pick up an item", 4, 10, false, "Pick up: bend at the knees and waist down toward the ground, close the hand as if grasping an item, then rise back up holding it. Clear down-then-up motion."},
	{"carry", "搬运", "互动", "walk while carrying a load", 6, 8, true, "Carrying walk loop: walking cycle with both arms held forward or up bearing a load, slightly leaned back to balance the weight, shorter steps. Looping burdened walk."},
	{"push", "推动", "互动", "push a heavy object forward", 6, 8, true, "Pushing loop: leaning hard forward with both arms extended against an object, legs driving with alternating steps, straining. Looping effortful push."},
	{"pull", "拉动", "互动", "pull a heavy object backward", 6, 8, true, "Pulling loop: leaning back with both arms drawn in gripping something, legs stepping backward and digging in, straining. Looping effortful pull."},
	{"open", "打开", "互动", "open a door or chest", 4, 10, false, "Open: reach forward toward a handle, grip and pull or push it open with a turning motion, lean in. Clear reach-and-open action with the arm doing the work."},
	{"eat", "进食", "互动", "eating food", 4, 8, false, "Eating: raise a hand to the mouth as if holding food, take a bite with a small head tilt, lower the hand, chew. Clear hand-to-mouth motion."},
	{"drink", "饮用", "互动", "drinking from a cup", 4, 8, false, "Drinking: raise a hand to the mouth as if holding a cup, tip the head back to drink, then lower. Clear raise-tip-lower motion."},
	{"read", "阅读", "互动", "reading a held book", 4, 6, true, "Reading loop: both hands held out front as if holding an open book, head tilted down scanning, occasional small head shift or page turn. Looping calm study."},
	{"dig", "挖掘", "互动", "digging with a shovel", 6, 8, true, "Digging loop: thrust a shovel down into the ground, scoop, lift and toss the dirt aside, return. Looping dig cycle with clear down-scoop-toss phases."},
	{"mine", "采矿", "互动", "swinging a pickaxe to mine", 6, 10, true, "Mining loop: raise a pickaxe overhead, swing it down hard into rock with an impact recoil, lift back up. Looping swing cycle with a clear strike frame."},
	{"chop", "砍伐", "互动", "chopping with an axe", 6, 10, true, "Chopping loop: raise an axe up and back, swing it down into a target with an impact jolt, recover up. Looping chop cycle with a clear strike frame."},
	{"fish", "钓鱼", "互动", "fishing, cast and wait", 5, 6, true, "Fishing loop: holding a rod out front, a slow gentle bob of the line and small body sway while waiting, occasional tiny tug check. Looping patient wait."},
}

// presetByName은 이름 → 프리셋 빠른 조회용 인덱스입니다.
var presetByName = func() map[string]PresetInfo {
	m := make(map[string]PresetInfo, len(Presets))
	for _, p := range Presets {
		m[p.Name] = p
	}
	return m
}()

// ListPresets는 100개 상황 키워드 카탈로그를 반환합니다 (Hint 제외, json:"-").
func ListPresets() []PresetInfo {
	out := make([]PresetInfo, len(Presets))
	copy(out, Presets)
	return out
}

// PresetByName은 이름에 해당하는 프리셋을 반환합니다.
func PresetByName(name string) (PresetInfo, bool) {
	p, ok := presetByName[strings.ToLower(strings.TrimSpace(name))]
	return p, ok
}

// MotionHint는 상태 이름에 대응하는 모션 가이드를 반환합니다 (카탈로그 기반).
// 8방향 세트는 상태명이 "attack-south" / "attack-north-east"처럼 방향 접미사가
// 붙으므로, 정확히 일치하지 않으면 방향 접미사를 떼고 베이스 키워드로 재조회합니다.
func MotionHint(stateName string) string {
	key := strings.ToLower(strings.TrimSpace(stateName))
	if p, ok := presetByName[key]; ok {
		return p.Hint
	}
	if base := stripDirectionSuffix(key); base != key {
		if p, ok := presetByName[base]; ok {
			return p.Hint
		}
	}
	return ""
}

// stripDirectionSuffix는 상태명 끝의 방향 키 접미사("-south" 등)를 제거합니다.
// 복합 방향 키("-south-east")를 단일 키("-east")보다 먼저 검사해
// "attack-south-east"가 "attack-south"로 잘못 잘리는 것을 방지합니다.
func stripDirectionSuffix(name string) string {
	// 1차: 복합 키 (하이픈 포함, 예: south-east)
	for _, d := range Directions {
		if strings.Contains(d.Key, "-") {
			if suffix := "-" + d.Key; strings.HasSuffix(name, suffix) {
				return strings.TrimSuffix(name, suffix)
			}
		}
	}
	// 2차: 단일 키 (south/north/east/west)
	for _, d := range Directions {
		if !strings.Contains(d.Key, "-") {
			if suffix := "-" + d.Key; strings.HasSuffix(name, suffix) {
				return strings.TrimSuffix(name, suffix)
			}
		}
	}
	return name
}
