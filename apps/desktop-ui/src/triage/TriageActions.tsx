// 검색 결과 하나를 어떻게 할지 정하는 버튼들.
//
// Quick Search 팝업의 결과 패널과 메인 창의 검색 상세가 같은 컴포넌트를 쓴다. 같은
// 결정("학습할래요"/"삭제")을 두 곳에서 내리는데 버튼이 따로 있으면 문구도 동작도 조용히
// 갈라진다 — 특히 문장은 단어를 고르기 전엔 학습에 넣을 수 없다는 규칙이 한쪽에만 남기 쉽다.

import { useState } from "react";
import { api, type LearnKind, type TriageResult } from "../api/client";
import "./TriageActions.css";

export interface TriageActionsProps {
  captureId: string;
  /** 해석 전에는 모른다 — 그때는 아직 판정할 수 없으므로 버튼을 그리지 않는다. */
  learnKind?: LearnKind;
  /** AI 해석의 상태(queued/running/done/failed). 실패는 별도의 두 선택지로 간다. */
  jobStatus?: string;
  /** 실패한 해석을 다시 걸었을 때. 호출자가 상세를 다시 읽게 한다. */
  onRetried?: () => void;
  /**
   * 문장에서 모르는 단어를 고르러 가는 동작. 팝업에서는 메인 창을 열고, 메인 창에서는
   * 아래 선택 패널을 편다. 어느 쪽이든 이 컴포넌트가 알 필요는 없다.
   */
  onPickWords?: () => void;
  /** 학습 등록·삭제가 끝난 뒤. 목록 새로고침 등은 호출자 몫. */
  onDone?: (result: TriageResult) => void;
  /** 사용자가 단어/문장 판정을 고친 뒤. 호출자가 상세를 다시 읽게 한다. */
  onKindChanged?: (learnKind: LearnKind) => void;
}

export default function TriageActions({
  captureId,
  learnKind,
  jobStatus,
  onPickWords,
  onDone,
  onKindChanged,
  onRetried,
}: TriageActionsProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const run = async (action: "learn" | "discard") => {
    setBusy(true);
    setError(null);
    try {
      const result =
        action === "learn" ? await api.learnSearch(captureId) : await api.discardSearch(captureId);
      setNote(
        action === "discard"
          ? "삭제했어요"
          : result.cards_created > 0
            ? `학습 목록에 담고 복습 카드 ${result.cards_created}장을 만들었어요`
            : "학습 목록에 담았어요",
      );
      onDone?.(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  // 해석이 실패하면 판정이 없어 아래 버튼들이 전부 그려지지 않는다 — 그대로 두면 이 검색은
  // 다시 해석할 수도, 지울 수도 없이 목록에 남는다. 실패에는 실패의 두 선택지가 있다.
  if (jobStatus === "failed") {
    const retry = async () => {
      setBusy(true);
      setError(null);
      try {
        await api.retrySearch(captureId);
        setNote("다시 해석하고 있어요…");
        onRetried?.();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setBusy(false);
      }
    };
    return (
      <div className="tri">
        <div className="tri-buttons">
          <button className="tri-primary" onClick={() => void retry()} disabled={busy}>
            다시 해석
          </button>
          <button className="tri-ghost" onClick={() => void run("discard")} disabled={busy}>
            삭제
          </button>
        </div>
        {note && <p className="tri-note">{note}</p>}
        {error && <p className="tri-error">⚠ {error}</p>}
      </div>
    );
  }

  if (!learnKind) return null;

  // 판정을 뒤집는다. 서버가 단어/문장을 자동으로 정하는 건 "단축키로 즉시"를 지키기
  // 위해서지만(D1), 문장을 단어로 잘못 부르면 단어 선택 흐름을 통째로 건너뛴다.
  const flipKind = async () => {
    const next: LearnKind = learnKind === "sentence" ? "word" : "sentence";
    setBusy(true);
    setError(null);
    try {
      await api.setSearchKind(captureId, next);
      setNote(next === "sentence" ? "문장으로 바꿨어요" : "단어로 바꿨어요");
      onKindChanged?.(next);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="tri">
      <div className="tri-buttons">
        {learnKind === "sentence" ? (
          <button className="tri-primary" onClick={onPickWords} disabled={busy || !onPickWords}>
            모르는 단어 고르기
          </button>
        ) : (
          <button className="tri-primary" onClick={() => void run("learn")} disabled={busy}>
            학습할래요
          </button>
        )}
        <button className="tri-ghost" onClick={() => void run("discard")} disabled={busy}>
          삭제
        </button>
      </div>
      <button className="tri-link" onClick={() => void flipKind()} disabled={busy}>
        {learnKind === "sentence" ? "문장이 아니라 단어예요" : "단어가 아니라 문장이에요"}
      </button>
      {note && <p className="tri-note">{note}</p>}
      {error && <p className="tri-error">⚠ {error}</p>}
    </div>
  );
}
