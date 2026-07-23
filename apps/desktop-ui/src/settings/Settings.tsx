// Settings 화면(#17, PRD §10.7/§15.8).
//
// 설정을 두 계층으로 나눠 보여준다(ADR-0004 부록):
//  - preferences(동작 정책·운영 튜닝): 알림 허용, 아침/저녁 복습 시간 → 편집·저장(app_settings).
//    저장만 하고 실제 알림 구동은 #18 소관.
//  - effective(부트스트랩/시작 옵션): AI provider·모델·DB 경로·주소·API key 유무 →
//    .env로만 설정하는 읽기전용. API key는 값이 아니라 설정 여부만 표시.

import { useCallback, useEffect, useState } from "react";
import { open as openDialog, save as saveDialog } from "@tauri-apps/plugin-dialog";
import { readTextFile, writeTextFile } from "@tauri-apps/plugin-fs";
import {
  api,
  ApiError,
  BACKUP_TABLE_KEYS,
  type BackupImportResult,
  type BackupSnapshot,
  type EffectiveConfig,
  type SettingsPreferences,
} from "../api/client";
import "./Settings.css";

const TABLE_LABEL_KO: Record<string, string> = {
  knowledge_items: "단어/용어",
  captures: "검색 기록",
  explanations: "해석",
  capture_items: "검색-단어 연결",
  learner_items: "학습 상태",
  review_cards: "복습 카드",
  review_logs: "복습 기록",
  lookup_jobs: "해석 작업 상태",
  review_card_candidates: "카드 후보",
};

// 백업 파일이 최소한 우리가 아는 스냅샷 형태인지 느슨하게 확인한다(엄격한 검증은
// 서버의 ValidateSnapshotVersion/ValidateLookupJobs가 담당 — 여기서는 명백히
// 엉뚱한 파일을 API 왕복 없이 걸러내는 정도로 충분하다).
function looksLikeBackupSnapshot(value: unknown): value is BackupSnapshot {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { version?: unknown }).version === "number"
  );
}

function summarizeSnapshot(snapshot: BackupSnapshot): Array<{ key: string; count: number }> {
  return BACKUP_TABLE_KEYS.filter((key) => Array.isArray(snapshot[key])).map((key) => ({
    key,
    count: (snapshot[key] as unknown[]).length,
  }));
}

// ApiError는 서버의 실제 에러 메시지를 담고 있다(request()가 {"error":"..."}를 파싱) —
// 413(대용량 거부)·400(지원 안 하는 version 등)을 사람이 읽을 수 있는 문장으로 보여준다.
function describeError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 413) return "파일이 너무 커요. 더 작은 파일을 써 주세요.";
    return err.message;
  }
  return err instanceof Error ? err.message : String(err);
}

type BackupPhase =
  | { kind: "idle" }
  | { kind: "exporting" }
  | { kind: "exported"; path: string }
  | { kind: "reading-import" }
  | { kind: "confirm-import"; path: string; snapshot: BackupSnapshot }
  | { kind: "importing"; path: string; snapshot: BackupSnapshot }
  | { kind: "imported"; result: BackupImportResult }
  | { kind: "backing-up" }
  | { kind: "backed-up"; path: string; sizeBytes: number }
  | { kind: "error"; message: string };

export default function Settings() {
  const [prefs, setPrefs] = useState<SettingsPreferences | null>(null);
  const [effective, setEffective] = useState<EffectiveConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.getSettings();
      setPrefs(res.preferences);
      setEffective(res.effective);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const update = (patch: Partial<SettingsPreferences>) => {
    setSaved(false);
    setPrefs((p) => (p ? { ...p, ...patch } : p));
  };

  const save = async () => {
    if (!prefs) return;
    setSaving(true);
    setError(null);
    try {
      const res = await api.updateSettings(prefs);
      setPrefs(res.preferences);
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const [backup, setBackup] = useState<BackupPhase>({ kind: "idle" });

  const handleExport = useCallback(async () => {
    const path = await saveDialog({
      defaultPath: "neulsang-backup.json",
      filters: [{ name: "JSON", extensions: ["json"] }],
    });
    if (!path) return; // 사용자 취소
    setBackup({ kind: "exporting" });
    try {
      const snapshot = await api.exportBackup();
      await writeTextFile(path, JSON.stringify(snapshot, null, 2));
      setBackup({ kind: "exported", path });
    } catch (err) {
      setBackup({ kind: "error", message: describeError(err) });
    }
  }, []);

  const handlePickImportFile = useCallback(async () => {
    const path = await openDialog({
      multiple: false,
      filters: [{ name: "JSON", extensions: ["json"] }],
    });
    if (!path || Array.isArray(path)) return; // 사용자 취소(다중 선택은 여기서 안 씀)
    setBackup({ kind: "reading-import" });
    try {
      const text = await readTextFile(path);
      let parsed: unknown;
      try {
        parsed = JSON.parse(text);
      } catch {
        setBackup({ kind: "error", message: "이 파일은 읽을 수 없어요." });
        return;
      }
      if (!looksLikeBackupSnapshot(parsed)) {
        setBackup({ kind: "error", message: "이 파일은 백업 파일이 아닌 것 같아요." });
        return;
      }
      setBackup({ kind: "confirm-import", path, snapshot: parsed });
    } catch (err) {
      setBackup({ kind: "error", message: describeError(err) });
    }
  }, []);

  const handleConfirmImport = useCallback(async () => {
    if (backup.kind !== "confirm-import") return;
    const { path, snapshot } = backup;
    setBackup({ kind: "importing", path, snapshot });
    try {
      const result = await api.importBackup(snapshot);
      setBackup({ kind: "imported", result });
    } catch (err) {
      setBackup({ kind: "error", message: describeError(err) });
    }
  }, [backup]);

  const handleCancelImport = useCallback(() => setBackup({ kind: "idle" }), []);

  const handleBackupFile = useCallback(async () => {
    const path = await saveDialog({
      defaultPath: "neulsang-backup.db",
      filters: [{ name: "SQLite DB", extensions: ["db", "sqlite"] }],
    });
    if (!path) return; // 사용자 취소
    setBackup({ kind: "backing-up" });
    try {
      const result = await api.backupToFile(path);
      setBackup({ kind: "backed-up", path: result.path, sizeBytes: result.size_bytes });
    } catch (err) {
      setBackup({ kind: "error", message: describeError(err) });
    }
  }, []);

  if (loading && !prefs) {
    return <div className="st-msg">불러오는 중…</div>;
  }

  return (
    <div className="st">
      <h1>설정</h1>

      {error && <p className="st-error">{error}</p>}

      {prefs && (
        <section className="st-panel">
          <h2>알림·복습</h2>
          <label className="st-row st-check">
            <input
              type="checkbox"
              checked={prefs.notifications_enabled}
              onChange={(e) => update({ notifications_enabled: e.target.checked })}
            />
            <span>알림 허용</span>
          </label>
          <label className="st-row">
            <span className="st-label">아침 복습 시간</span>
            <input
              type="time"
              value={prefs.morning_review_time}
              onChange={(e) => update({ morning_review_time: e.target.value })}
            />
          </label>
          <label className="st-row">
            <span className="st-label">저녁 복습 시간</span>
            <input
              type="time"
              value={prefs.evening_review_time}
              onChange={(e) => update({ evening_review_time: e.target.value })}
            />
          </label>
          <div className="st-actions">
            <button className="st-save" onClick={() => void save()} disabled={saving}>
              {saving ? "저장 중…" : "저장"}
            </button>
            {saved && <span className="st-saved">저장됨 ✓</span>}
          </div>
          <p className="st-note">
            저장한 시간대에 맞춰 복습 알림이 떠요(트레이 배지·OS 알림, #18). 검색 결과가
            준비되면 즉시 다른 알림도 떠요.
          </p>
        </section>
      )}

      {effective && (
        <section className="st-panel">
          <h2>기본 값(파일에서 읽음·읽기전용)</h2>
          <p className="st-note">
            아래 값은 앱을 시작할 때 정해져요. 바꾸려면 <code>.env</code> 파일을 고치고
            앱을 다시 켜야 해요 — 개발 중에는 프로젝트 폴더의 <code>.env</code>, 설치한
            앱에서는 <code>~/Library/Application Support/neulsang/.env</code> 파일이에요.
          </p>
          <ReadRow label="AI 서비스" value={effective.ai_provider} />
          {effective.gemini_model && <ReadRow label="모델" value={effective.gemini_model} />}
          <ReadRow
            label="API 키"
            value={effective.api_key_configured ? "설정됨" : "미설정"}
            tone={effective.api_key_configured ? "ok" : "warn"}
          />
          <ReadRow label="데이터 저장 위치" value={effective.db_path} mono />
          <ReadRow label="주소" value={effective.addr} mono />
        </section>
      )}

      <section className="st-panel">
        <h2>백업·복원</h2>
        <p className="st-note">
          내보내기를 하면 지금까지 배운 기록을 전부 파일로 저장해요. 가져오기를 하면 그
          파일을 다시 불러오는데, 이미 있는 내용은 다시 만들지 않으니 여러 번 눌러도
          안전해요.
        </p>
        <div className="st-actions">
          <button
            className="st-save"
            onClick={() => void handleExport()}
            disabled={backup.kind === "exporting"}
          >
            {backup.kind === "exporting" ? "내보내는 중…" : "기록 내보내기"}
          </button>
          <button
            className="st-save"
            onClick={() => void handlePickImportFile()}
            disabled={backup.kind === "reading-import" || backup.kind === "importing"}
          >
            {backup.kind === "reading-import" ? "읽는 중…" : "기록 가져오기"}
          </button>
          <button
            className="st-save"
            onClick={() => void handleBackupFile()}
            disabled={backup.kind === "backing-up"}
          >
            {backup.kind === "backing-up" ? "백업하는 중…" : "전체 백업하기"}
          </button>
        </div>

        {backup.kind === "exported" && (
          <p className="st-saved">내보내기 완료 ✓ {backup.path}</p>
        )}
        {backup.kind === "backed-up" && (
          <p className="st-saved">
            백업 완료 ✓ {backup.path} ({(backup.sizeBytes / 1024).toFixed(0)} KB)
          </p>
        )}
        {backup.kind === "error" && <p className="st-error">{backup.message}</p>}

        {backup.kind === "confirm-import" && (
          <ImportConfirm
            path={backup.path}
            snapshot={backup.snapshot}
            onConfirm={() => void handleConfirmImport()}
            onCancel={handleCancelImport}
          />
        )}

        {(backup.kind === "importing" || backup.kind === "imported") && (
          <ImportResultView phase={backup} />
        )}
      </section>
    </div>
  );
}

function ReadRow({
  label,
  value,
  mono,
  tone,
}: {
  label: string;
  value: string;
  mono?: boolean;
  tone?: "ok" | "warn";
}) {
  return (
    <div className="st-row st-read">
      <span className="st-label">{label}</span>
      <span className={`st-value${mono ? " st-mono" : ""}${tone ? ` st-${tone}` : ""}`}>{value}</span>
    </div>
  );
}

// 가져오기 전 요약 확인(RW-09: "import 전에 파일 version/요약을 보여주고 사용자 확인을
// 받는다"). 실제 version/status 검증은 서버가 하므로(ValidateSnapshotVersion 등,
// RW-04) 여기서는 그 결과를 사람이 이해할 수 있게 미리 보여주는 역할만 한다.
function ImportConfirm({
  path,
  snapshot,
  onConfirm,
  onCancel,
}: {
  path: string;
  snapshot: BackupSnapshot;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const summary = summarizeSnapshot(snapshot);
  return (
    <div className="st-import-confirm">
      <p className="st-note">
        <strong>{path}</strong> (version {snapshot.version})
      </p>
      <ul className="st-import-summary">
        {summary.map(({ key, count }) => (
          <li key={key}>
            {TABLE_LABEL_KO[key] ?? key}: {count}건
          </li>
        ))}
      </ul>
      <p className="st-note">
        이미 있는 내용은 그대로 두고, 새로운 내용만 더해요. 계속할까요?
      </p>
      <div className="st-actions">
        <button className="st-save" onClick={onConfirm}>
          가져오기 진행
        </button>
        <button className="st-cancel" onClick={onCancel}>
          취소
        </button>
      </div>
    </div>
  );
}

function ImportResultView({
  phase,
}: {
  phase: { kind: "importing" } | { kind: "imported"; result: BackupImportResult };
}) {
  if (phase.kind === "importing") {
    return <p className="st-note">가져오는 중…</p>;
  }
  const rows = BACKUP_TABLE_KEYS.filter((key) => phase.result[key] !== undefined);
  return (
    <div className="st-import-result">
      <p className="st-saved">가져오기 완료 ✓</p>
      <table className="st-import-table">
        <thead>
          <tr>
            <th></th>
            <th>추가</th>
            <th>병합</th>
            <th>갱신</th>
            <th>건너뜀</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((key) => {
            const r = phase.result[key]!;
            return (
              <tr key={key}>
                <td>{TABLE_LABEL_KO[key] ?? key}</td>
                <td>{r.inserted}</td>
                <td>{r.merged}</td>
                <td>{r.updated}</td>
                <td>{r.skipped}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
