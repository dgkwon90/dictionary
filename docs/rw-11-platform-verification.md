# RW-11 지원 플랫폼 릴리스 검증

- 목적: [`remaining-work.md`](planning/remaining-work.md) RW-11의 완료 조건("지원한다고 문서화한 모든 OS/arch에 설치 가능한 산출물과 smoke 기록")을 실제 결과로 채운다.
- 범위: **macOS(arm64/x86_64) + Windows 11만.** Linux는 [`ADR-0009`](adr/ADR-0009-platform-scope-linux-deferred.md)로 이번 릴리스에서 제외.
- 이 문서는 (A) 자동화 가능해 Claude가 이미 CLI로 실행·확인한 항목과, (B) 실제 GUI 클릭이 필요해 사람이 손으로 확인해야 하는 항목을 분리한다. (B)는 체크박스만 두고 실행은 사용자 몫이다.

## A. 자동화 검증 완료 (macOS arm64, 이 세션에서 실행)

빌드: `npm --prefix apps/desktop-ui run tauri build -- --debug --bundles app`

- [x] `.app` 번들에 `desktopd`(사이드카)와 `neulsang`(메인) 둘 다 포함되고 애드혹 서명됨 — `codesign --verify --deep --strict` 통과, 둘 다 `Signature=adhoc`, arm64 Mach-O 확인.
- [x] 번들을 그대로 실행(`Contents/MacOS/neulsang` 직접 fork, cwd 무관) → 번들 안의 `desktopd`가 자식으로 spawn됨(로그로 pid 확인) → **사용자 config `.env`(`~/Library/Application Support/neulsang/.env`)에서 실제 Gemini 설정을 로드**(로그에 `"using Gemini explainer"`, mock 폴백 아님 — #25 config 주입이 번들에서도 동작함을 재확인).
- [x] `GET /healthz` → `{"status":"ok"}`, 토큰 없이도 200(RW-01 예외 규칙대로).
- [x] `GET /v1/inbox?status=new` 토큰 없이 요청 → 401(RW-01 인증이 번들 실행에서도 실제로 걸림, 정적 코드 검토가 아니라 살아있는 프로세스에 대고 확인).
- [x] 메인 프로세스에 `SIGTERM` 직접 전송(터미널에서 `kill`, 즉 Dock/트레이 Quit을 거치지 않는 비정상 종료 시나리오) → 2초 내 `desktopd`까지 완전히 종료, 고아 프로세스 없음. 로그를 보니 이 경로는 **RW-03의 `graceful_stop()`이 아니라 #13의 부모-사망 watchdog**(`os.Getppid()` 재입양 감지, 2초 폴링)가 잡은 것으로 확인됨(`"parent process gone; shutting down"` 로그). 즉 이 세션이 검증한 것은 "비정상 종료에도 고아가 안 남는다"는 결과이지, RW-03가 만든 SIGTERM→유예→강제종료 경로 자체는 아니다.
- [x] 트레이 Quit → `graceful_stop()` 배선은 **정적 코드 추적**으로 확인: `tray.rs`의 `"quit" => app.exit(0)` → `lib.rs`의 `RunEvent::Exit => app.state::<Desktopd>().shutdown()` → `sidecar.rs`의 `shutdown()`이 `graceful_stop()` 호출. `graceful_stop()` 자체의 SIGTERM→유예→SIGKILL 승격 로직은 Rust 단위 테스트 2종(`graceful_stop_lets_a_well_behaved_child_exit_on_sigterm`, `graceful_stop_escalates_to_sigkill_after_grace`)으로 이미 통과 확인(RW-03). **다만 "트레이 Quit 메뉴를 실제로 클릭했을 때" 전체 체인이 눈으로 보이는 동작까지 확인한 것은 아니다** — 아래 B의 macOS 체크리스트 항목으로 남겨둠.

## B. 사람이 GUI로 직접 확인해야 하는 항목

### B-1. macOS (arm64 — 이 Mac, 지금 바로 가능)

빌드는 A와 동일(`tauri build -- --debug --bundles app`) 또는 GitHub Release의 `.dmg`(현재 `v0.1.0` 릴리스는 RW-01~RW-10 이전 커밋 기준이라 이번 하드닝 반영 안 됨 — 이 하드닝까지 검증하려면 로컬 디버그 빌드나 새 태그 릴리스가 필요, 아래 "다음 결정" 참고).

- [ ] `.app`을 더블클릭(또는 `open`)으로 실행 → 트레이 아이콘이 메뉴바에 나타난다.
- [ ] 트레이 메뉴에서 Quick Search/Inbox/Today Review/Dashboard/Settings 각각 클릭 → 해당 화면으로 전환된다.
- [ ] 트레이 Quit 클릭 → 앱 종료 후 터미널에서 `ps aux | grep desktopd`로 **고아 프로세스가 없는지 확인**(이번엔 정상 종료 경로라 RW-03의 graceful_stop 로그도 확인 가능 — 콘솔 앱이나 `log stream`으로 "desktopd stopped" 확인 권장).
- [ ] 아무 앱에서나 `Cmd+Shift+E` → Quick Search 팝업이 뜬다. 텍스트 복사 후 다시 호출 → 클립보드 내용이 자동 삽입된다.
- [ ] 한글 발음(예: "스테일") 입력 → 후보(stale 등) 표시 → 선택 → 해석 결과가 뜬다(RW-08).
- [ ] Settings에서 알림 허용 체크 + 아침/저녁 시각 저장 → macOS가 알림 권한을 물으면 허용.
- [ ] 아무 단어나 검색 → 잠시 후 "검색 결과 준비" OS 알림 배너가 뜬다(팝업을 닫은 상태에서 확인 — #18).
- [ ] 알림 배너 클릭 → 앱이 활성화되고 관련 화면(Inbox 등)으로 이동한다(#29, `RunEvent::Reopen`).
- [ ] Settings의 "백업·복원"에서 내보내기(JSON) → 파일 저장 다이얼로그가 뜨고 파일이 생성된다.
- [ ] 같은 화면에서 그 파일을 다시 가져오기 → 요약(버전, 테이블별 행 수) 확인 화면이 뜨고, 확인 버튼을 눌러야 실제로 반영된다(RW-09/RW-04).
- [ ] "SQLite 파일로 백업" → 파일 생성 확인.

### B-2. macOS (x86_64 / Intel — 다른 Mac이 있다면)

동일한 B-1 체크리스트를 Intel Mac에서 반복. 로컬에 Intel Mac이 없다면 GitHub Release의 x86_64 자산으로 대체 가능(단, 위와 같은 이유로 최신 하드닝을 검증하려면 새 태그가 필요).

### B-3. Windows 11

이 세션은 Windows 실기기/에뮬레이터에 접근할 수 없어 전부 사람 확인이 필요하다.

- [ ] GitHub Actions가 빌드한 `.msi`/`.exe`로 설치. Windows Defender SmartScreen 경고가 뜨는 것은 **의도된 동작**(미서명, backlog #32에서 수용).
- [ ] 트레이(작업표시줄 시스템 트레이)에 아이콘이 나타나고 메뉴가 동작한다.
- [ ] `Ctrl+Shift+E` 글로벌 단축키로 Quick Search가 뜬다.
- [ ] sidecar(`desktopd.exe`) 수명주기: 작업 관리자로 `neulsang.exe` 종료(작업 관리자에서 "작업 끝내기") 시 `desktopd.exe`도 함께 정리되는지 확인. **알려진 갭(RW-03에 문서화)**: Windows는 SIGTERM 상당 기능이 없어 Rust 쪽 graceful 경로가 macOS와 다르게 즉시 kill로 동작 — 다만 macOS에서 확인한 것과 유사하게 #13 watchdog(`os.Getppid()` 폴링)이 안전망으로 동작해야 한다. 이 항목이 이번 검증의 핵심 — Windows에서 watchdog 재입양 감지가 실제로 되는지는 한 번도 실기기로 확인된 적 없다.
- [ ] API key: `.env` 대신 Windows Credential Manager 경로(#26, `zalando/go-keyring`의 Windows 백엔드)가 실제로 동작하는지. 이 백엔드는 지금까지 **한 번도 실기기에서 검증된 적 없다**(macOS Keychain만 테스트됨).
- [ ] OS 알림(토스트)이 뜨는지, 클릭 시 앱이 포커스되는지(단, `RunEvent::Reopen`은 macOS 전용 `#[cfg(target_os="macos")]`라 Windows에서는 **알림 클릭 시 앱 포커스+화면 이동 자체가 아직 구현 안 됨** — 이 검증에서 확인되면 별도 후속 이슈로 등록).

## 다음 결정이 필요한 지점

이번에 병합한 RW-01~RW-10 하드닝(토큰 인증, graceful shutdown, backup v2 등)은 `v0.1.0` 태그(커밋 `fad4fe3`) **이후** 커밋이라, 기존 GitHub Release 자산에는 반영되어 있지 않다. Windows/Intel Mac에서 이 하드닝까지 포함해 검증하려면 새 릴리스 태그(예: `v0.1.1`)를 만들어 GitHub Actions가 새로 빌드하게 해야 한다 — 태그 push는 공개 릴리스를 만드는 행위라 사용자 승인 없이 진행하지 않는다.
