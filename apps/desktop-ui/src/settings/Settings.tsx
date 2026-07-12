// Settings 화면(#17, PRD §10.7/§15.8).
//
// 설정을 두 계층으로 나눠 보여준다(ADR-0004 부록):
//  - preferences(동작 정책·운영 튜닝): 알림 허용, 아침/저녁 복습 시간 → 편집·저장(app_settings).
//    저장만 하고 실제 알림 구동은 #18 소관.
//  - effective(부트스트랩/시작 옵션): AI provider·모델·DB 경로·주소·API key 유무 →
//    .env로만 설정하는 읽기전용. API key는 값이 아니라 설정 여부만 표시.

import { useCallback, useEffect, useState } from "react";
import {
  api,
  type EffectiveConfig,
  type SettingsPreferences,
} from "../api/client";
import "./Settings.css";

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

  if (loading && !prefs) {
    return <div className="st-msg">불러오는 중…</div>;
  }

  return (
    <div className="st">
      <h1>Settings</h1>

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
          <p className="st-note">복습 시간 설정은 저장되며, 실제 알림은 이후 업데이트에서 동작합니다(#18).</p>
        </section>
      )}

      {effective && (
        <section className="st-panel">
          <h2>환경(.env로 설정 · 읽기전용)</h2>
          <p className="st-note">
            아래는 프로세스 시작 시 환경변수로 결정됩니다. 변경하려면 저장소 루트의 <code>.env</code>를
            수정하고 앱을 재시작하세요.
          </p>
          <ReadRow label="AI provider" value={effective.ai_provider} />
          {effective.gemini_model && <ReadRow label="모델" value={effective.gemini_model} />}
          <ReadRow
            label="API key"
            value={effective.api_key_configured ? "설정됨" : "미설정"}
            tone={effective.api_key_configured ? "ok" : "warn"}
          />
          <ReadRow label="DB 경로" value={effective.db_path} mono />
          <ReadRow label="주소" value={effective.addr} mono />
        </section>
      )}
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
