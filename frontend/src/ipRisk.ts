export interface IPRiskMatch {
  group: string;
  term: string;
  replacement: string;
}

interface IPRiskRule {
  group: string;
  aliases: string[];
  zh: string;
  en: string;
}

const IP_RISK_RULES: IPRiskRule[] = [
  { group: "Gundam", aliases: ["机动战士高达0079", "高达0079", "機動戰士鋼彈", "机动战士高达", "mobile suit gundam", "gundam", "高达", "鋼彈"], zh: "原创复古科幻机甲", en: "an original retro sci-fi mecha" },
  { group: "Amuro Ray", aliases: ["阿姆罗·雷", "阿姆羅·雷", "amuro ray", "阿姆罗", "阿姆羅", "amuro"], zh: "原创机甲驾驶员", en: "an original mecha pilot" },
  { group: "Pokemon", aliases: ["pokemon", "pokémon", "宝可梦", "寶可夢", "皮卡丘", "pikachu"], zh: "原创奇幻伙伴生物", en: "an original fantasy companion creature" },
  { group: "Mario", aliases: ["super mario", "超级马里奥", "超級瑪利歐", "马里奥", "瑪利歐", "mario"], zh: "原创卡通冒险角色", en: "an original cartoon adventure character" },
  { group: "Zelda", aliases: ["the legend of zelda", "塞尔达传说", "薩爾達傳說", "塞尔达", "薩爾達", "zelda", "link"], zh: "原创奇幻冒险角色", en: "an original fantasy adventure character" },
  { group: "Dragon Ball", aliases: ["dragon ball", "龙珠", "龍珠", "孙悟空", "孫悟空", "goku"], zh: "原创热血格斗角色", en: "an original martial-arts hero" },
  { group: "Naruto", aliases: ["火影忍者", "漩涡鸣人", "漩渦鳴人", "naruto"], zh: "原创忍者冒险角色", en: "an original ninja adventure character" },
  { group: "One Piece", aliases: ["one piece", "海贼王", "海賊王", "路飞", "魯夫", "luffy"], zh: "原创海洋冒险角色", en: "an original seafaring adventure character" },
  { group: "Evangelion", aliases: ["neon genesis evangelion", "新世纪福音战士", "新世紀福音戰士", "evangelion", "eva初号机", "eva初號機"], zh: "原创生物机械巨人", en: "an original biomechanical giant" },
  { group: "Transformers", aliases: ["transformers", "变形金刚", "變形金剛", "optimus prime", "擎天柱"], zh: "原创可变形机械角色", en: "an original transforming mechanical character" },
  { group: "Ultraman", aliases: ["ultraman", "奥特曼", "奧特曼", "ウルトラマン"], zh: "原创巨大光之英雄", en: "an original giant light-powered hero" },
  { group: "Kamen Rider", aliases: ["kamen rider", "假面骑士", "假面騎士", "仮面ライダー"], zh: "原创装甲变身英雄", en: "an original armored transformation hero" },
  { group: "Sonic", aliases: ["sonic the hedgehog", "刺猬索尼克", "索尼克", "sonic"], zh: "原创高速卡通角色", en: "an original high-speed cartoon character" },
  { group: "Final Fantasy", aliases: ["final fantasy", "最终幻想", "最終幻想"], zh: "原创奇幻角色", en: "an original fantasy character" },
  { group: "Marvel", aliases: ["marvel", "漫威", "spider-man", "spiderman", "蜘蛛侠", "蜘蛛俠", "iron man", "钢铁侠", "鋼鐵人", "avengers", "复仇者联盟"], zh: "原创超级英雄角色", en: "an original superhero character" },
  { group: "DC", aliases: ["dc comics", "batman", "蝙蝠侠", "蝙蝠俠", "superman", "超人", "wonder woman", "神奇女侠"], zh: "原创超级英雄角色", en: "an original superhero character" },
  { group: "Star Wars", aliases: ["star wars", "星球大战", "星際大戰", "darth vader", "达斯维达", "绝地武士", "jedi"], zh: "原创太空歌剧角色", en: "an original space-opera character" },
  { group: "Harry Potter", aliases: ["harry potter", "哈利波特", "霍格沃茨", "hogwarts"], zh: "原创魔法学院角色", en: "an original magic-academy character" },
  { group: "Disney", aliases: ["mickey mouse", "米老鼠", "米奇", "donald duck", "唐老鸭", "冰雪奇缘", "frozen", "elsa"], zh: "原创家庭动画角色", en: "an original family-animation character" },
  { group: "Minecraft", aliases: ["minecraft", "我的世界", "creeper", "苦力怕"], zh: "原创方块世界角色", en: "an original block-world character" },
  { group: "Doraemon", aliases: ["doraemon", "哆啦a梦", "哆啦A梦", "多啦a夢", "机器猫"], zh: "原创未来伙伴机器人", en: "an original futuristic companion robot" },
  { group: "Hello Kitty", aliases: ["hello kitty", "凯蒂猫", "凱蒂貓"], zh: "原创可爱吉祥物角色", en: "an original cute mascot character" },
];

const sortedRules = IP_RISK_RULES.flatMap((rule) =>
  rule.aliases.map((alias) => ({ rule, alias })),
).sort((a, b) => b.alias.length - a.alias.length);

export function findIPRiskTerms(text: string, chinese = true): IPRiskMatch[] {
  const source = text.trim();
  if (!source) return [];
  const lower = source.toLocaleLowerCase();
  const seenGroups = new Set<string>();
  const matches: IPRiskMatch[] = [];
  for (const { rule, alias } of sortedRules) {
    if (seenGroups.has(rule.group)) continue;
    const match = aliasPattern(alias).exec(lower);
    if (!match) continue;
    seenGroups.add(rule.group);
    const start = match.index;
    matches.push({
      group: rule.group,
      term: source.slice(start, start + alias.length),
      replacement: chinese ? rule.zh : rule.en,
    });
  }
  return matches;
}

export function neutralizeIPDescription(text: string, chinese = true): string {
  let result = text;
  const replacedGroups = new Set<string>();
  for (const { rule, alias } of sortedRules) {
    const pattern = aliasPattern(alias, true);
    if (!pattern.test(result)) continue;
    pattern.lastIndex = 0;
    const replacement = replacedGroups.has(rule.group) ? "" : chinese ? rule.zh : rule.en;
    result = result.replace(pattern, replacement);
    replacedGroups.add(rule.group);
  }
  return result
    .replace(/\s{2,}/g, " ")
    .replace(/\s+([,.;，。；])/g, "$1")
    .trim();
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function aliasPattern(alias: string, global = false): RegExp {
  const escaped = escapeRegExp(alias);
  const asciiOnly = /^[\x20-\x7E]+$/.test(alias);
  return new RegExp(asciiOnly ? `\\b${escaped}\\b` : escaped, global ? "giu" : "iu");
}
