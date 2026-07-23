// 여러 화면에서 같이 쓰는 한글 표시 라벨. 서버가 주는 값(영문 enum 등)은 그대로 두고
// 사람이 읽는 문구만 여기서 통일한다 — 같은 개념에 서로 다른 번역이 생기지 않게 한다.

const CARD_TYPE_LABELS: Record<string, string> = {
  meaning: "뜻 맞추기",
  reverse: "영어로 떠올리기",
  cloze: "빈칸 채우기",
  context: "쓰임 고르기",
  sentence_translation: "문장 해석하기",
};

export function cardTypeLabel(cardType: string): string {
  return CARD_TYPE_LABELS[cardType] ?? cardType;
}

const CATEGORY_LABELS: Record<string, string> = {
  backend: "백엔드",
  infra: "인프라",
  database: "데이터베이스",
  network: "네트워크",
  general: "일반",
};

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category;
}
