// Package watchdog는 desktopd가 Tauri 셸의 사이드카로 실행될 때 부모(셸) 프로세스가
// 사라지면 스스로 종료하도록 한다.
//
// 셸이 정상 종료되면 셸이 자식(desktopd)을 kill하지만, 셸이 SIGKILL·패닉 등으로
// 비정상 종료되면 자식이 고아로 남아 48989 포트·SQLite를 붙든 채 살아남는다. 이때
// 커널이 고아를 init(1)/launchd에 재입양하므로 `getppid()`가 최초 부모와 달라진다 —
// 이 변화를 감지해 종료 컨텍스트를 취소한다(SIGINT와 동일한 graceful shutdown 경로).
//
// PID 재사용에 안전하다: 특정 PID의 생존을 probe하지 않고 재입양(부모 PID 변화)만 본다.
//
// 플랫폼: macOS(launchd로 재입양)·Linux(init/subreaper로 재입양)에서 동작한다. Windows는
// 고아 재입양이 없어 os.Getppid()가 변하지 않으므로 이 방식이 감지하지 못한다 — Windows
// 지원 시 셸에서 Job Object(kill-on-close)로 대체해야 한다(현재 대상은 macOS 우선).
package watchdog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// ParentPIDEnv가 설정돼 있을 때만 감시를 활성화한다(사이드카 모드 게이트).
// 셸이 자신의 PID를 넣어 desktopd를 실행한다. 값 자체는 진단·게이트 용도다.
const ParentPIDEnv = "NEULSANG_PARENT_PID"

const pollInterval = 2 * time.Second

// WatchParent는 감시가 활성화된 경우, 부모가 사라지면 함께 취소되는 파생 컨텍스트를
// 돌려준다. 비활성(환경변수 미설정)이면 입력 컨텍스트를 그대로 돌려준다.
func WatchParent(ctx context.Context, log *slog.Logger) context.Context {
	return watchParent(ctx, log, os.Getppid, pollInterval)
}

// watchParent는 getppid·간격을 주입받는 테스트 가능한 코어다.
func watchParent(
	ctx context.Context,
	log *slog.Logger,
	getppid func() int,
	interval time.Duration,
) context.Context {
	if _, ok := parentPIDFromEnv(); !ok {
		return ctx
	}
	original := getppid()

	wctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				cancel()
				return
			case <-ticker.C:
				if parentGone(original, getppid) {
					log.Warn("parent process gone; shutting down", "original_ppid", original)
					cancel()
					return
				}
			}
		}
	}()
	return wctx
}

// parentGone은 재입양(현재 부모 PID가 최초 부모와 달라짐)으로 부모 소멸을 판정한다.
func parentGone(original int, getppid func() int) bool {
	return getppid() != original
}

// parentPIDFromEnv는 게이트 환경변수를 파싱한다. 없거나 형식이 틀리면 (0,false).
func parentPIDFromEnv() (int, bool) {
	raw := os.Getenv(ParentPIDEnv)
	if raw == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
